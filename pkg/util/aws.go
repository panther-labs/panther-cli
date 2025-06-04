package util

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/pkg/errors"
)

const maxRetries = 5

//nolint:forbidigo
func GetAWSConfig(ctx context.Context, region, accessKeyID, secretAccessKey, sessionToken string) (aws.Config, error) {
	if accessKeyID == "UNSET" && secretAccessKey == "UNSET" {
		log.Println("No credentials provided, attempting to load AWS config from environment.")
		defaultRetryer := config.WithRetryer(func() aws.Retryer {
			return retry.AddWithMaxAttempts(retry.NewStandard(), maxRetries)
		})
		awsCfg, err := config.LoadDefaultConfig(ctx, defaultRetryer, config.WithRegion(region))
		if err != nil {
			LogWarnf("Failed to load AWS config from environment: %v", err)
			LogWarnln("Attempting to load AWS configuration from credentials.")
		}

		return awsCfg, nil
	}

	log.Println("Attempting to load AWS configuration from specified credentials.")
	awsCfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, sessionToken),
		),
		config.WithRetryer(func() aws.Retryer {
			return retry.AddWithMaxAttempts(retry.NewStandard(), maxRetries)
		}),
		config.WithRegion(region),
	)
	if err != nil {
		return aws.Config{}, errors.Wrap(err, "failed to load AWS config")
	}

	return awsCfg, nil
}
