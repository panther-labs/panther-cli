package config

import (
	"strings"
)

type SnowflakeConfigType int

const (
	SnowflakeConfigTypeNewAccount SnowflakeConfigType = iota
	SnowflakeConfigTypeExistingAccount
)

//nolint:lll
type SnowflakeConfig struct {
	ConfigType            SnowflakeConfigType            `yaml:"-"                     validate:"required,oneof=NewAccountConfig ExistingAccountConfig"`
	NewAccountConfig      NewSnowflakeAccountConfig      `yaml:"NewAccountConfig"      validate:"required_without=ExistingAccountConfig"`
	ExistingAccountConfig ExistingSnowflakeAccountConfig `yaml:"ExistingAccountConfig" validate:"required_without=NewAccountConfig"`
}

//nolint:lll
type SnowflakeOrgConfig struct {
	AccountLocator         string `yaml:"AccountLocator"         validate:"required"`
	AccountRegion          string `yaml:"AccountRegion"          validate:"required,lowercase,validPantherRegion"`
	OrgAdminUsername       string `yaml:"OrgAdminUsername"       validate:"required"`
	OrgAdminPrivateKey     string `yaml:"-"                      validate:"required"                              json:"-"` // This will be populated from the file
	OrgAdminPrivateKeyPath string `yaml:"OrgAdminPrivateKeyPath" validate:"required"`
}

//nolint:lll
type ExistingSnowflakeAccountConfig struct {
	AccountLocatorURL             string `yaml:"AccountLocatorURL"             validate:"required,url"`
	URL                           string `yaml:"Url"                           validate:"required,url"`
	Edition                       string `yaml:"Edition"                       validate:"required,oneof=STANDARD ENTERPRISE BUSINESS_CRITICAL"`
	Region                        string `yaml:"SnowflakeRegion"               validate:"required,lowercase,validateSnowflakeRegion"`
	PantherAccountAdminRSAKeyPath string `yaml:"PantherAccountAdminRSAKeyPath" validate:"required"`
}

func (c ExistingSnowflakeAccountConfig) IsEmpty() bool {
	return c == ExistingSnowflakeAccountConfig{}
}

func (c ExistingSnowflakeAccountConfig) GetAWSRegion() string {
	region := strings.ReplaceAll(c.Region, "_", "-")
	region = strings.TrimLeft(region, "aws_")
	return c.Region
}

//nolint:lll
type NewSnowflakeAccountConfig struct {
	OrgConfig SnowflakeOrgConfig `yaml:"OrgConfig" validate:"required"`

	AccountName        string `yaml:"SnowflakeAccountName" validate:"required,validAcctName"`
	Edition            string `yaml:"SnowflakeEdition"     validate:"required,oneof=STANDARD ENTERPRISE BUSINESS_CRITICAL"` // if PantherEdition!=Enterprise, SnowflakeEdition can be whatever
	AdminUsername      string `yaml:"AdminUsername"        validate:"required,validAdminName"`
	AdminPassword      string `yaml:"AdminPassword"        validate:"required,min=32"`
	AdminEmail         string `yaml:"AdminEmail"           validate:"required,email"`
	AdminUserFirstName string `yaml:"AdminUserFirstName"   validate:"required"`
	AdminUserLastName  string `yaml:"AdminUserLastName"    validate:"required"`
	Region             string `yaml:"-"                    validate:"required,lowercase,validPantherRegion"`
}

func (c NewSnowflakeAccountConfig) IsEmpty() bool {
	return c == NewSnowflakeAccountConfig{}
}

func (n NewSnowflakeAccountConfig) GetSnowflakeRegion() string {
	region := strings.ReplaceAll(n.Region, "-", "_")
	return "aws_" + region
}
