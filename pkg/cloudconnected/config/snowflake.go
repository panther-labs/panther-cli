package config

import "strings"

type SnowflakeOrgConfig struct {
	AccountLocator         string `yaml:"AccountLocator"         validate:"required"`
	AccountRegion          string `yaml:"AccountRegion"          validate:"required,validPantherRegion"`
	OrgAdminUsername       string `yaml:"OrgAdminUsername"       validate:"required"`
	OrgAdminPrivateKey     string `yaml:"-"` // This will be populated from the file
	OrgAdminPrivateKeyPath string `yaml:"OrgAdminPrivateKeyPath" validate:"required"`
}

//nolint:lll
type NewAccountConfig struct {
	SnowflakeAccountName string `yaml:"SnowflakeAccountName" validate:"required,validateSnowflakeAccountName"`
	SnowflakeEdition     string `yaml:"SnowflakeEdition"     validate:"required,oneof=STANDARD ENTERPRISE BUSINESS_CRITICAL"` // if PantherEdition!=Enterprise, SnowflakeEdition can be whatever

	AdminUsername      string `yaml:"AdminUsername"      validate:"required,validAdminName"`
	AdminPassword      string `yaml:"AdminPassword"      validate:"required,min=32"`
	AdminEmail         string `yaml:"AdminEmail"         validate:"required,email"`
	AdminUserFirstName string `yaml:"AdminUserFirstName" validate:"required"`
	AdminUserLastName  string `yaml:"AdminUserLastName"  validate:"required"`

	DesiredPantherAccountName string   `yaml:"DesiredPantherAccountName" validate:"required"`
	PantherEdition            string   `yaml:"PantherEdition"            validate:"required,oneof=ENTERPRISE ESSENTIALS"` // if PantherEdition==Enterprise, SnowflakeEdition must be Enterprise
	PantherRegion             string   `yaml:"PantherRegion"             validate:"required,validPantherRegion"`          // this informs which Snowflake region we use, currently AWS-only
	IpAddressAllowList        []string `yaml:"IpAddressAllowList"        validate:"omitempty,dive,ip"`                    // Optional list of allowed IP addresses
}

func (n NewAccountConfig) GetSnowflakeRegion() string {
	region := strings.ReplaceAll(n.PantherRegion, "-", "_")
	return "aws_" + region
}
