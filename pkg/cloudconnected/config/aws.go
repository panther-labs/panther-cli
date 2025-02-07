package config

type AWSConfig struct {
	AccessKeyID     string `yaml:"AccessKeyID"     validate:"required"`
	SecretAccessKey string `yaml:"SecretAccessKey" validate:"required"`
	SessionToken    string `yaml:"SessionToken"`
	Region          string `yaml:"Region"          validate:"required,validPantherRegion"`

	CloudFormationConfig           CloudFormationConfig           `yaml:"CloudFormationConfig"           validate:"required"`
	DomainCertificateConfiguration DomainCertificateConfiguration `yaml:"DomainCertificateConfiguration" validate:"required"`
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
