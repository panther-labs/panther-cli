package config

import "strings"

type SnowflakeOrgConfig struct {
	AccountLocator     string `yaml:"AccountLocator"     validate:"required"`
	AccountRegion      string `yaml:"AccountRegion"      validate:"required,validPantherRegion"`
	OrgAdminUsername   string `yaml:"OrgAdminUsername"   validate:"required"`
	OrgAdminPrivateKey string `yaml:"OrgAdminPrivateKey" validate:"required"`
}

//nolint:lll
type NewAccountConfig struct {
	AccountName      string `yaml:"AccountName"      validate:"required,validAcctName"`
	AdminUsername    string `yaml:"AdminUsername"    validate:"required,validAdminName"`
	AdminPassword    string `yaml:"AdminPassword"    validate:"required,min=32"`
	AdminEmail       string `yaml:"AdminEmail"       validate:"required,email"`
	SnowflakeEdition string `yaml:"SnowflakeEdition" validate:"required,oneof=STANDARD ENTERPRISE BUSINESS_CRITICAL"` // if PantherEdition!=Enterprise, SnowflakeEdition can be whatever
	PantherEdition   string `yaml:"PantherEdition"   validate:"required,oneof=ENTERPRISE ESSENTIALS"`                 // if PantherEdition==Enterprise, SnowflakeEdition must be Enterprise
	PantherRegion    string `yaml:"PantherRegion"    validate:"required,validPantherRegion"`                          // this informs which Snowflake region we use, currently AWS-only
}

func (n NewAccountConfig) GetSnowflakeRegion() string {
	region := strings.ReplaceAll(n.PantherRegion, "-", "_")
	return "aws_" + region
}
