package aws

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/util"

	"github.com/aws/smithy-go"
)

type CloudFormation struct {
	ctx    context.Context
	cfg    config.AWSConfig
	awsCfg aws.Config
}

func NewCloudFormation(ctx context.Context, cfg config.AWSConfig) (*CloudFormation, error) {
	awsCfg, err := util.GetAWSConfig(ctx, cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize AWS client config")
	}

	return &CloudFormation{
		ctx,
		cfg,
		awsCfg,
	}, nil
}

func (c *CloudFormation) ApplyDeploymentRole() error {
	templateContent, err := util.LoadContent(c.ctx, c.cfg.CloudFormationConfig.DeploymentRoleTemplateURL)
	if err != nil {
		return errors.Wrap(err, "failed to fetch CloudFormation template for deployment role")
	}

	log.Printf("Deploying deployment role from '%s'", c.cfg.CloudFormationConfig.DeploymentRoleTemplateURL)

	stackName := c.cfg.CloudFormationConfig.DeploymentRoleStackName

	parameters := []types.Parameter{
		{
			ParameterKey:   aws.String("DeploymentRoleName"),
			ParameterValue: aws.String(c.cfg.CloudFormationConfig.DeploymentRoleName),
		},
		{
			ParameterKey:   aws.String("IdentityAccountId"),
			ParameterValue: aws.String(c.cfg.CloudFormationConfig.IdentityAccountId),
		},
		{
			ParameterKey:   aws.String("OpsAccountId"),
			ParameterValue: aws.String(c.cfg.CloudFormationConfig.OpsAccountId),
		},
	}

	return c.applyCloudFormationTemplate(templateContent, stackName, parameters)
}

func (c *CloudFormation) ApplyPreDeploymentTools() error {
	templateURL := c.cfg.CloudFormationConfig.PreDeploymentToolsTemplateURL
	if strings.Contains(templateURL, "%s") {
		// if we locally provided a template, no need to Sprintf
		templateURL = fmt.Sprintf(templateURL, c.cfg.Region)
	}

	templateContent, err := util.LoadContent(c.ctx, templateURL)
	if err != nil {
		return errors.Wrap(err, "failed to fetch CloudFormation template for pre-deployment tools")
	}

	log.Printf("Deploying pre-deployment tools from '%s'", templateURL)

	parameters := []types.Parameter{} // this template doesn't take parameters

	return c.applyCloudFormationTemplate(
		templateContent,
		c.cfg.CloudFormationConfig.PreDeploymentToolsStackName,
		parameters,
	)
}

func (c *CloudFormation) applyCloudFormationTemplate(
	templateContent string,
	stackName string,
	parameters []types.Parameter,
) error {
	cfnClient := cloudformation.NewFromConfig(c.awsCfg, func(o *cloudformation.Options) {
		o.Region = c.cfg.Region
	})

	// Call CreateStack or UpdateStack based on stack existence
	_, err := cfnClient.DescribeStacks(c.ctx, &cloudformation.DescribeStacksInput{
		StackName: &stackName,
	})
	if err != nil {
		// Assume the stack does not exist and create it
		_, err = cfnClient.CreateStack(c.ctx, &cloudformation.CreateStackInput{
			StackName:    &stackName,
			TemplateBody: &templateContent,
			Capabilities: []types.Capability{
				types.CapabilityCapabilityNamedIam,
				types.CapabilityCapabilityAutoExpand,
			},
			Parameters: parameters,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to create stack (%s)", stackName)
		}
		log.Printf("Stack creation initiated: %s\n", stackName)
	} else {
		// If stack exists, update it
		_, err = cfnClient.UpdateStack(c.ctx, &cloudformation.UpdateStackInput{
			StackName:    &stackName,
			TemplateBody: &templateContent,
			Capabilities: []types.Capability{
				types.CapabilityCapabilityNamedIam,
				types.CapabilityCapabilityAutoExpand,
			},
		})
		if err != nil {
			// Check if the error is a "No updates are to be performed" error
			var apiErr smithy.APIError
			ok := errors.As(err, &apiErr)
			if ok && apiErr.ErrorCode() == "ValidationError" && apiErr.ErrorMessage() == "No updates are to be performed." {
				log.Println("No updates to perform on the stack.")
			} else {
				return errors.Wrapf(err, "failed to update stack (%s)", stackName)
			}
		} else {
			log.Printf("Stack update initiated: %s\n", stackName)
		}
	}

	if err := monitorStackProgress(c.ctx, cfnClient, stackName); err != nil {
		return errors.Wrapf(err, "failed to monitor stack progress (%s)", stackName)
	}

	return nil
}

// monitorStackProgress polls the stack state until it reaches a terminal state.
func monitorStackProgress(ctx context.Context, cfnClient *cloudformation.Client, stackName string) error {
	for {
		resp, err := cfnClient.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
			StackName: &stackName,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to describe stack (%s)", stackName)
		}

		if len(resp.Stacks) == 0 {
			return errors.Errorf("stack not found: %s", stackName)
		}

		stack := resp.Stacks[0]
		log.Printf("Current stack status: %s\n", stack.StackStatus)

		// Check for terminal states
		switch stack.StackStatus {
		case types.StackStatusCreateComplete, types.StackStatusUpdateComplete:
			log.Println("Stack operation completed successfully.")
			return nil
		case types.StackStatusCreateFailed,
			types.StackStatusRollbackFailed,
			types.StackStatusRollbackComplete,
			types.StackStatusDeleteFailed:
			return errors.Errorf("stack operation failed with status: %s", stack.StackStatus)
		}

		// Wait before polling again
		time.Sleep(10 * time.Second)
	}
}
