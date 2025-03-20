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
	"github.com/k0kubun/pp/v3"
	"github.com/pkg/errors"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/snowflake"
	"github.com/panther-labs/panther-cli/pkg/util"
)

// SnowflakeValidationResponse represents the successful response from the Snowflake credential validation lambda
type SnowflakeValidationResponse struct {
	StatusCode int                     `json:"statusCode"`
	Headers    map[string]string       `json:"headers"`
	Body       SnowflakeValidationBody `json:"body"`
}

// SnowflakeValidationBody represents the body field in the validation response
type SnowflakeValidationBody struct {
	Message  string `json:"message"`
	CredsARN string `json:"credsArn"`
}

// SnowflakeValidationError represents the error response from the Snowflake credential validation lambda
type SnowflakeValidationError struct {
	ErrorMessage string   `json:"errorMessage"`
	ErrorType    string   `json:"errorType"`
	RequestID    string   `json:"requestId"`
	StackTrace   []string `json:"stackTrace"`
}

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

	// Try to unmarshal as error response first
	var errorResponse SnowflakeValidationError
	if err := json.Unmarshal(result.Payload, &errorResponse); err == nil && errorResponse.ErrorMessage != "" {
		return "", errors.Errorf("validation failed with error: %s", errorResponse.ErrorMessage)
	}

	// If not an error, try to unmarshal as success response
	var response SnowflakeValidationResponse
	if err := json.Unmarshal(result.Payload, &response); err != nil {
		return "", errors.Wrapf(err, "couldn't parse payload from validate response: %s", string(result.Payload))
	}

	util.LogDebugf("unmarshalled PantherSnowflakeCredentialBootstrap response: %s", pp.Sprint(response))

	// Check for successful status code
	if response.StatusCode == 200 {
		log.Printf("Validation succeeded with message: %s", response.Body.Message)
		if response.Body.CredsARN != "" {
			return response.Body.CredsARN, nil
		}
	}

	return "", errors.Errorf(
		"unexpected response from Snowflake credential validation lambda, report this to your Panther rep: %s",
		string(result.Payload),
	)
}
