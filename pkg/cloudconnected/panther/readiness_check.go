package panther

import (
	"context"
	"encoding/json"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/util"
	"github.com/pkg/errors"
)

type ReadinessCheck struct {
	ctx    context.Context
	cfg    config.AWSConfig
	awsCfg aws.Config
}

func NewReadinessCheck(ctx context.Context, cfg config.AWSConfig) (*ReadinessCheck, error) {
	awsCfg, err := util.GetAWSConfig(ctx, cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize AWS client config")
	}

	return &ReadinessCheck{
		ctx,
		cfg,
		awsCfg,
	}, nil
}

const (
	readinessCheckLambdaName = "PantherReadinessCheck"
	readinessCheckPayload    = "{}"
)

// We're not making this type more structured in order to leave more flexibility
// as more checks may be added in the future.
type ReadinessCheckResult map[string]interface{}

func (r *ReadinessCheck) Exec() (ReadinessCheckResult, error) {
	// create new lambda client
	lambdaClient := lambda.NewFromConfig(r.awsCfg, func(o *lambda.Options) {
		o.Region = r.cfg.Region
	})

	payloadBytes, err := json.Marshal(readinessCheckPayload)
	if err != nil {
		return ReadinessCheckResult{}, errors.Wrap(err, "failed to marshal input payload for Lambda")
	}

	input := &lambda.InvokeInput{
		FunctionName:   aws.String(readinessCheckLambdaName),
		Payload:        payloadBytes,
		InvocationType: types.InvocationTypeRequestResponse, // Use synchronous invocation
	}

	log.Println("Invoking readiness check - this may take a moment...")

	// Call the Lambda function
	output, err := lambdaClient.Invoke(r.ctx, input)
	if err != nil {
		return ReadinessCheckResult{}, errors.Wrapf(
			err,
			"failed to invoke %s lambda function",
			readinessCheckLambdaName,
		)
	}

	// Check if the Lambda invocation was successful
	if output.FunctionError != nil && *output.FunctionError != "" {
		return ReadinessCheckResult{}, errors.Errorf(
			"Lambda function error: %s - payload=\n%s",
			*output.FunctionError,
			string(output.Payload),
		)
	}

	// Unmarshal the Lambda response
	var result ReadinessCheckResult
	err = json.Unmarshal(output.Payload, &result)
	if err != nil {
		return ReadinessCheckResult{}, errors.Wrap(err, "failed to unmarshal Lambda response payload")
	}

	return result, nil
}
