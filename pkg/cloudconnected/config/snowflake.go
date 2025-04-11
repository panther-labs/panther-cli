package config

import (
	"strings"
)

//nolint:lll
type SnowflakeConfig struct {
	OrgConfig             SnowflakeOrgConfig             `yaml:"OrgConfig"             validate:"required_without=ExistingAccountConfigg"`
	ExistingAccountConfig ExistingSnowflakeAccountConfig `yaml:"ExistingAccountConfig" validate:"required_without=OrgConfig"`
}

//nolint:lll
type SnowflakeOrgConfig struct {
	AccountLocator         string `yaml:"AccountLocator"         validate:"required"`
	AccountRegion          string `yaml:"AccountRegion"          validate:"required,validPantherRegion"`
	OrgAdminUsername       string `yaml:"OrgAdminUsername"       validate:"required"`
	OrgAdminPrivateKey     string `yaml:"-"` // This will be populated from the file
	OrgAdminPrivateKeyPath string `yaml:"OrgAdminPrivateKeyPath" validate:"required"`
}

//nolint:lll
type ExistingSnowflakeAccountConfig struct {
	AccountLocatorURL             string `yaml:"AccountLocatorURL"             validate:"required,url"`
	URL                           string `yaml:"Url"                           validate:"required,url"`
	Edition                       string `yaml:"Edition"                       validate:"required,oneof=STANDARD ENTERPRISE BUSINESS_CRITICAL"`
	Region                        string `yaml:"SnowflakeRegion"               validate:"required,validateSnowflakeRegion"`
	PantherAccountAdminRSAKeyPath string `yaml:"PantherAccountAdminRSAKeyPath" validate:"required"`
}

//nolint:lll
type NewSnowflakeAccountConfig struct {
	AccountName        string `yaml:"SnowflakeAccountName" validate:"required,validAcctName"`
	Edition            string `yaml:"SnowflakeEdition"     validate:"required,oneof=STANDARD ENTERPRISE BUSINESS_CRITICAL"` // if PantherEdition!=Enterprise, SnowflakeEdition can be whatever
	AdminUsername      string `yaml:"AdminUsername"        validate:"required,validAdminName"`
	AdminPassword      string `yaml:"AdminPassword"        validate:"required,min=32"`
	AdminEmail         string `yaml:"AdminEmail"           validate:"required,email"`
	AdminUserFirstName string `yaml:"AdminUserFirstName"   validate:"required"`
	AdminUserLastName  string `yaml:"AdminUserLastName"    validate:"required"`
}

//nolint:lll
type PantherAccountConfig struct {
	DesiredPantherAccountName string   `yaml:"DesiredPantherAccountName" validate:"required"`
	Edition                   string   `yaml:"PantherEdition"            validate:"required,oneof=ENTERPRISE ESSENTIALS"` // if PantherEdition==Enterprise, SnowflakeEdition must be Enterprise
	Region                    string   `yaml:"PantherRegion"             validate:"required,validPantherRegion"`          // this informs which Snowflake region we use, currently AWS-only
	IpAddressAllowList        []string `yaml:"IpAddressAllowList"        validate:"omitempty,dive,ip"`                    // Optional list of allowed IP addresses
}

//nolint:lll
type NewAccountConfig struct {
	SnowflakeAccountName string `yaml:"SnowflakeAccountName" validate:"required,validAcctName"`
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
