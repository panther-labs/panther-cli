package config

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/pkg/errors"

	"github.com/panther-labs/panther-cli/pkg/util"
)

type AWSConfig struct {
	AccessKeyID     string `yaml:"AccessKeyID"     validate:"required"`
	SecretAccessKey string `yaml:"SecretAccessKey" validate:"required"`
	SessionToken    string `yaml:"SessionToken"`
	Region          string `yaml:"Region"          validate:"required,validPantherRegion"`

	CloudFormationConfig           CloudFormationConfig           `yaml:"CloudFormationConfig"           validate:"required"`
	DomainCertificateConfiguration DomainCertificateConfiguration `yaml:"DomainCertificateConfiguration" validate:"required"`
}

// GetAWSAccountID retrieves the AWS account ID associated with the provided credentials
// using the AWS STS service. This is useful to dynamically identify the AWS account.
func (a *AWSConfig) GetAWSAccountID(ctx context.Context) (string, error) {
	// Create AWS config from credentials
	awsCfg, err := util.GetAWSConfig(ctx, a.AccessKeyID, a.SecretAccessKey, a.SessionToken)
	if err != nil {
		return "", errors.Wrap(err, "failed to initialize AWS client config")
	}

	// Set region from config
	awsCfg.Region = a.Region

	// Create STS client
	stsClient := sts.NewFromConfig(awsCfg)

	// Call GetCallerIdentity to get account information
	result, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", errors.Wrap(err, "failed to get AWS account ID")
	}

	// Return the account ID from the response
	return *result.Account, nil
}

func (a *AWSConfig) MustGetAWSAccountID() string {
	accountID, err := a.GetAWSAccountID(context.Background())
	if err != nil {
		log.Fatalf("failed to get AWS account ID: %v", err)
	}
	return accountID
}

//nolint:lll
type CloudFormationConfig struct {
	IdentityAccountId             string `yaml:"IdentityAccountId"             validate:"required"`
	OpsAccountId                  string `yaml:"OpsAccountId"                  validate:"required"`
	DeploymentRoleName            string `yaml:"DeploymentRoleName"                                default:"PantherDeploymentRole"`
	DeploymentRoleTemplateURL     string `yaml:"DeploymentRoleTemplateURL"                         default:"https://panther-public-cloudformation-templates.s3.us-west-2.amazonaws.com/panther-deployment-role/latest/template.yml"`
	DeploymentRoleStackName       string `yaml:"DeploymentRoleStackName"                           default:"PantherDeploymentRoleStack"`
	PreDeploymentToolsTemplateURL string `yaml:"PreDeploymentToolsTemplateURL"                     default:"https://panther-public-cloudformation-templates.s3.us-west-2.amazonaws.com/panther-preflight-tools-%s/latest/template.yml"`
	PreDeploymentToolsStackName   string `yaml:"PreDeploymentToolsStackName"                       default:"PantherPreDeploymentTools"`
}

type DomainCertificateConfiguration struct {
	PantherSubdomain string `yaml:"PantherSubdomain" validate:"required,fqdn"`
}
