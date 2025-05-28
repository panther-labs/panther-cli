package util

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/pkg/errors"
)

const maxRetries = 5

//nolint:forbidigo
func GetAWSConfig(ctx context.Context, region, accessKeyID, secretAccessKey, sessionToken string) (aws.Config, error) {
	awsCfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, sessionToken),
		),
		config.WithRetryer(func() aws.Retryer {
			return retry.AddWithMaxAttempts(retry.NewStandard(), maxRetries)
		}),
	)
	if err != nil {
		return aws.Config{}, errors.Wrap(err, "failed to load AWS config")
	}

	return awsCfg, nil
}
