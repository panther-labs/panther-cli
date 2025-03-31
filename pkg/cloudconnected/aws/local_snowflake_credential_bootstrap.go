package aws

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/snowflake"
	"github.com/panther-labs/panther-cli/pkg/rsapem"
	"github.com/panther-labs/panther-cli/pkg/util"
	"github.com/pkg/errors"
)

const (
	snowflakeSecretName = "panther-managed-accountadmin-secret"
)

type SnowflakeCredentialSecret struct {
	Account     string `json:"account"     `
	Username    string `json:"user"        `
	PrivateKey1 string `json:"privateKey1" `
	Host        string `json:"host"        `
	Port        string `json:"port"        `
	secretARN   string `json:"-"           `
}

type LocalSnowflakeCredentialBootstrap struct {
	secretsManager *secretsmanager.Client
}

func NewLocalSnowflakeCredentialBootstrap(
	ctx context.Context,
	awsConfig config.AWSConfig,
) (*LocalSnowflakeCredentialBootstrap, error) {
	cfg, err := util.GetAWSConfig(ctx, awsConfig.AccessKeyID, awsConfig.SecretAccessKey, awsConfig.SessionToken)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get AWS config")
	}

	secretsManager := secretsmanager.NewFromConfig(cfg)

	return &LocalSnowflakeCredentialBootstrap{
		secretsManager: secretsManager,
	}, nil
}

// WriteSecret writes the secret to AWS Secrets Manager as a JSON payload.
func (l *LocalSnowflakeCredentialBootstrap) WriteSecret(
	ctx context.Context,
	createAccountResult snowflake.CreateAccountResult,
) (string, error) {
	err := updateSnowflakeSecret(ctx, l.secretsManager, snowflakeSecretName, createAccountResult)
	if err != nil {
		return "", errors.Wrap(err, "failed to update Snowflake secret")
	}

	secret, err := l.ReadSecret(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to read secret")
	}

	return secret.secretARN, nil
}

// ReadSecret reads the secret from AWS Secrets Manager and parses it into a SnowflakeCredentialSecret.
func (l *LocalSnowflakeCredentialBootstrap) ReadSecret(
	ctx context.Context,
) (*SnowflakeCredentialSecret, error) {
	secret, err := l.secretsManager.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(snowflakeSecretName),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get secret from AWS Secrets Manager")
	}

	var secretValue SnowflakeCredentialSecret
	err = json.Unmarshal([]byte(*secret.SecretString), &secretValue)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal secret from AWS Secrets Manager")
	}

	secretValue.secretARN = *secret.ARN

	return &secretValue, nil
}

// ValidateSecret validates the secret by calling `ReadSecret` function to get the secret and then
// uses the returned `SnowflakeCredentialSecret` to connect to Snowflake and validate the credentials.
func (l *LocalSnowflakeCredentialBootstrap) ValidateSecret(ctx context.Context) error {
	secret, err := l.ReadSecret(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to read secret")
	}

	asRsaPrivateKey, err := rsapem.ParseRSAPEMPrivateKey(secret.PrivateKey1)
	if err != nil {
		return errors.Wrap(err, "failed to parse private key")
	}

	dsn := util.FormatSnowflakeDSNFromRSAKey(
		"", // we don't need to specify region here
		secret.Account,
		secret.Username,
		"ACCOUNTADMIN",
		asRsaPrivateKey,
	)

	db, err := sql.Open("snowflake", dsn)
	if err != nil {
		return errors.Wrapf(err, "failed to open connection to Snowflake host: '%s'", secret.Host)
	}

	defer func() {
		if err := db.Close(); err != nil {
			log.Fatalf("failed to close Snowflake connection: %v\n", err)
		}
	}()

	if err := db.Ping(); err != nil {
		return errors.Wrapf(err, "failed to ping Snowflake host: '%s'", secret.Host)
	}

	log.Printf("Successfully validated credentials with Snowflake host: '%s'", secret.Host)

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
