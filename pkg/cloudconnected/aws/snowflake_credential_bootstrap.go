package aws

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/pkg/errors"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/snowflake"
	"github.com/panther-labs/panther-cli/pkg/rsapem"
	"github.com/panther-labs/panther-cli/pkg/util"
)

const (
	snowflakeCredentialBootstrapLambdaName = "PantherSnowflakeCredentialBootstrap"
	snowflakeSecretName                    = "panther-managed-accountadmin-secret" // this is region specific
)

type PantherSnowflakeCredentialBootstrap struct {
	ctx    context.Context
	cfg    config.Config
	awsCfg aws.Config
}

func NewPantherSnowflakeCredentialBootstrap(
	ctx context.Context,
	cfg config.Config,
) (*PantherSnowflakeCredentialBootstrap, error) {
	awsCfg, err := util.GetAWSConfig(
		ctx,
		cfg.AWSConfig.AccessKeyID,
		cfg.AWSConfig.SecretAccessKey,
		cfg.AWSConfig.SessionToken,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize AWS client config")
	}

	return &PantherSnowflakeCredentialBootstrap{
		ctx,
		cfg,
		awsCfg,
	}, nil
}

func (p *PantherSnowflakeCredentialBootstrap) Exec(createAcctResult snowflake.CreateAccountResult) (string, error) {
	sm := secretsmanager.NewFromConfig(p.awsCfg, func(o *secretsmanager.Options) {
		o.Region = p.cfg.AWSConfig.Region
	})

	lambdaClient := lambda.NewFromConfig(p.awsCfg, func(o *lambda.Options) {
		o.Region = p.cfg.AWSConfig.Region
	})

	secretExists, err := snowflakeSecretExists(p.ctx, sm, snowflakeSecretName)
	if err != nil {
		return "", errors.Wrap(err, "failed to check if snowflake secret exists")
	}

	// run `validate` lambda function
	if !secretExists {
		if err := bootstrapCredentials(p.ctx, lambdaClient, createAcctResult.URL); err != nil {
			return "", errors.Wrap(err, "failed to bootstrap Snowflake credentials")
		}
	}

	// it takes a minute for secrets to be available, so we check it periodically in a loop until it's available
	const sleepDuration = 10 * time.Second
	const maxAttempts = 10
	for ii := range maxAttempts {
		if ii == maxAttempts-1 {
			return "", errors.Errorf(
				"max attempts reached waiting for '%s' lambda to bootstrap Snowflake secret",
				snowflakeCredentialBootstrapLambdaName,
			)
		}

		exists, err := snowflakeSecretExists(p.ctx, sm, snowflakeSecretName)
		if err != nil {
			return "", errors.Wrap(err, "failed to check if snowflake secret exists")
		}

		if !exists {
			log.Printf(
				"Snowflake secret not yet available, waiting %d second(s) before retrying",
				sleepDuration/time.Second,
			)
			time.Sleep(sleepDuration)
			continue
		}

		log.Println("Snowflake secret bootstrap complete")
		break
	}

	log.Println("Updating Snowflake secret with PANTHERACCOUNTADMIN password")
	if err := updateSnowflakeSecret(
		p.ctx,
		sm,
		snowflakeSecretName,
		createAcctResult,
	); err != nil {
		return "", errors.Wrap(err, "failed to write Snowflake secret")
	}

	credsARN, err := validateCredentials(p.ctx, lambdaClient)
	if err != nil {
		return "", errors.Wrap(err, "failed to validate Snowflake credentials")
	}

	log.Printf("Snowflake credentials validated successfully. Secret ARN: %s", credsARN)
	return credsARN, nil
}

func snowflakeSecretExists(ctx context.Context, sm *secretsmanager.Client, secretName string) (bool, error) {
	input := &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secretName),
	}

	_, err := sm.DescribeSecret(ctx, input)
	if err != nil {
		var notFoundErr *types.ResourceNotFoundException
		if errors.As(err, &notFoundErr) {
			return false, nil
		}

		return false, errors.Wrap(err, "failed to describe secret")
	}

	return true, nil
}

func bootstrapCredentials(ctx context.Context, lambdaClient *lambda.Client, host string) error {
	payload := map[string]string{
		"host": host,
	}
	payloadAsBytes, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to create payload for bootstrapping Snowflake credentials")
	}
	util.LogDebugf(
		"Invoking Snowflake credential bootstrap lambda (%s) with payload: '%s'",
		snowflakeCredentialBootstrapLambdaName,
		payloadAsBytes,
	)

	input := &lambda.InvokeInput{
		FunctionName:   aws.String(snowflakeCredentialBootstrapLambdaName),
		Payload:        payloadAsBytes,
		InvocationType: lambdatypes.InvocationTypeRequestResponse,
	}

	_, err = lambdaClient.Invoke(ctx, input)
	if err != nil {
		return errors.Wrapf(
			err,
			"failed to invoke '%s' lambda to bootstrap Snowflake credentials",
			snowflakeCredentialBootstrapLambdaName,
		)
	}

	return nil
}

func updateSnowflakeSecret(
	ctx context.Context,
	sm *secretsmanager.Client,
	secretName string,
	createAcctResult snowflake.CreateAccountResult,
) error {
	getSecretValueInput := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretName),
	}

	getSecretValueOutput, err := sm.GetSecretValue(ctx, getSecretValueInput)
	if err != nil {
		return errors.Wrap(err, "failed to get existing Snowflake secret")
	}

	var secretMap map[string]string
	if err := json.Unmarshal([]byte(*getSecretValueOutput.SecretString), &secretMap); err != nil {
		return errors.Wrap(err, "failed to unmarshal existing Snowflake secret")
	}

	fullyQualifiedAccount, err := createAcctResult.GetFullyQualifiedAccountName()
	if err != nil {
		return errors.Wrap(err, "failed to get fully qualified account name")
	}

	privateKey, err := rsapem.EncodeRSAPEMPrivateKey(createAcctResult.AdminRSAKey)
	if err != nil {
		return errors.Wrap(err, "encoding PrivateKey for PANTHERACCOUNTADMIN")
	}
	publicKey, err := rsapem.EncodeRSAPEMPublicKey(&createAcctResult.AdminRSAKey.PublicKey)
	if err != nil {
		return errors.Wrap(err, "encoding PublicKey for PANTHERACCOUNTADMIN")
	}
	createTime := time.Now().UTC().Format(time.RFC3339)

	// Update the secret values
	secretMap["host"] = createAcctResult.URL
	secretMap["account"] = fullyQualifiedAccount
	secretMap["privateKey1"] = privateKey
	secretMap["publicKey1"] = publicKey
	secretMap["privateKey1CreateTimestamp"] = createTime

	// Marshal the updated secret back to JSON
	updatedSecretString, err := json.Marshal(secretMap)
	if err != nil {
		return errors.Wrap(err, "failed to marshal updated Snowflake secret")
	}

	// write it back to Secrets Manager
	putSecretValueInput := &secretsmanager.PutSecretValueInput{
		SecretId:     aws.String(secretName),
		SecretString: aws.String(string(updatedSecretString)),
	}

	_, err = sm.PutSecretValue(ctx, putSecretValueInput)
	if err != nil {
		return errors.Wrap(err, "failed to write Snowflake secret")
	}

	return nil
}

func validateCredentials(ctx context.Context, lambdaClient *lambda.Client) (string, error) {
	payload := map[string]string{
		"validate": "true",
	}
	payloadAsBytes, err := json.Marshal(payload)
	if err != nil {
		return "", errors.Wrap(err, "failed to create payload for validating Snowflake credentials")
	}
	util.LogDebugf(
		"Invoking Snowflake credential validate lambda (%s) with payload: '%s'",
		snowflakeCredentialBootstrapLambdaName,
		payloadAsBytes,
	)

	input := &lambda.InvokeInput{
		FunctionName:   aws.String(snowflakeCredentialBootstrapLambdaName),
		Payload:        payloadAsBytes,
		InvocationType: lambdatypes.InvocationTypeRequestResponse,
	}

	result, err := lambdaClient.Invoke(ctx, input)
	if err != nil {
		return "", errors.Wrapf(
			err,
			"failed to invoke '%s' lambda to validate Snowflake credentials",
			snowflakeCredentialBootstrapLambdaName,
		)
	}

	util.LogDebugf("response payload: %s", string(result.Payload))

	// the old version of the snowflake cred bootstrapper lambda returns a string instead of json
	if strings.HasPrefix(string(result.Payload), "\"Validation succeeded for the secret.") {
		log.Println(string(result.Payload))
		return "", nil // Old version doesn't return creds_arn
	}

	// The new version of the snowflake cred bootstrapper lambda returns a json object like a good little lambda.
	var unmarshaledPayload map[string]interface{}
	if err := json.Unmarshal(result.Payload, &unmarshaledPayload); err != nil {
		return "", errors.Wrapf(err, "couldn't parse payload from validate response: %s", string(result.Payload))
	}

	// Check if unmarshaledPayload has a field called `errorMessage` - error case
	// The response body for a failing request should look like:
	// {
	// 	"errorMessage": "250003 (08001): 404 Not Found: post https://pantherlabs-zbrown_cc_provisioning_test34fart.snowflakecomputing.co...
	// 	"errorType": "InterfaceError",
	// 	"requestId": "2b6332ce-2361-4533-b183-f7f2ae7fbce8",
	// 	"stackTrace": [
	// 		<snipped stack trace>
	// 	]
	// }
	//
	// Details might differ.
	if errorMessage, ok := unmarshaledPayload["errorMessage"]; ok {
		return "", errors.Errorf("validation failed with error: %s", errorMessage)
	}

	// Check if we have a statusCode field and that it's 200 - success case
	// The response body for a successful request should look like:
	// {
	//   "statusCode": 200,
	//   "headers": { "Content-Type": "application/json" },
	//   "body": { "message": "Validation succeeded for the secret. <insert more info>" }
	//   "creds_arn": "arn:aws:secretsmanager:us-west-2:<snip>:secret:panther-managed-accountadmin-secret-<snip>"
	// }
	if statusCode, ok := unmarshaledPayload["statusCode"]; ok && statusCode.(float64) == 200 {
		// Extract and return creds_arn if available
		if credsARN, ok := unmarshaledPayload["creds_arn"]; ok {
			if credsARNStr, ok := credsARN.(string); ok {
				if body, ok := unmarshaledPayload["body"]; ok {
					if bodyMap, ok := body.(map[string]interface{}); ok {
						if message, ok := bodyMap["message"]; ok {
							log.Println(message)
						}
					}
				}
				return credsARNStr, nil
			}
		}
	}
	return "", errors.Errorf(
		"unexpected response from Snowflake credential validation lambda, report this to your Panther rep: %s",
		string(result.Payload),
	)
}
