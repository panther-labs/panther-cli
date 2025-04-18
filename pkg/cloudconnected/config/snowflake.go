package config

import (
	"log"
	"os"
	"strings"

	"github.com/k0kubun/pp/v3"
	"github.com/pkg/errors"
)

type SnowflakeConfigType string

const (
	SnowflakeConfigTypeNewAccount      SnowflakeConfigType = "SnowflakeConfigTypeNewAccount"
	SnowflakeConfigTypeExistingAccount SnowflakeConfigType = "SnowflakeConfigTypeExistingAccount"
)

//nolint:lll
type SnowflakeConfig struct {
	ConfigType            SnowflakeConfigType             `yaml:"-"                     validate:"required,oneof=SnowflakeConfigTypeNewAccount SnowflakeConfigTypeExistingAccount"`
	NewAccountConfig      *NewSnowflakeAccountConfig      `yaml:"NewAccountConfig"      validate:"required_without=ExistingAccountConfig,excluded_with=ExistingAccountConfig"`
	ExistingAccountConfig *ExistingSnowflakeAccountConfig `yaml:"ExistingAccountConfig" validate:"required_without=NewAccountConfig,excluded_with=NewAccountConfig"`
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
	AccountName                   string `yaml:"AccountName"                   validate:"required"`
	URL                           string `yaml:"URL"                           validate:"required,url"`
	Edition                       string `yaml:"Edition"                       validate:"required,oneof=STANDARD ENTERPRISE BUSINESS_CRITICAL"`
	Region                        string `yaml:"Region"                        validate:"required,lowercase,validPantherRegion"`
	PantherAccountAdminRSAKeyPath string `yaml:"PantherAccountAdminRSAKeyPath" validate:"required"`
}

func (e *ExistingSnowflakeAccountConfig) LoadPantherAccountAdminRSAKey() (string, error) {
	privateKey, err := os.ReadFile(e.PantherAccountAdminRSAKeyPath)
	if err != nil {
		log.Printf("failed to read PantherAccountAdminRSAKeyPath, error='%s', config section:\n%s", err, pp.Sprint(e))
		return "", err
	}
	return string(privateKey), nil
}

//nolint:lll
type NewSnowflakeAccountConfig struct {
	OrgConfig SnowflakeOrgConfig `yaml:"OrgConfig" validate:"required"`

	PantherAccountAdminRSAKeyOutputPath string `yaml:"PantherAccountAdminRSAKeyOutputPath" validate:"required"`

	AccountName        string `yaml:"SnowflakeAccountName" validate:"required,validAcctName"`
	Edition            string `yaml:"SnowflakeEdition"     validate:"required,oneof=STANDARD ENTERPRISE BUSINESS_CRITICAL"` // if PantherEdition!=Enterprise, SnowflakeEdition can be whatever
	AdminUsername      string `yaml:"AdminUsername"        validate:"required,validAdminName"`
	AdminPassword      string `yaml:"AdminPassword"        validate:"required,min=32"`
	AdminEmail         string `yaml:"-"                    validate:"required,email"`
	AdminUserFirstName string `yaml:"-"                    validate:"required"`
	AdminUserLastName  string `yaml:"-"                    validate:"required"`
	Region             string `yaml:"-"                    validate:"required,lowercase,validPantherRegion"`
}

func (n *NewSnowflakeAccountConfig) GetSnowflakeRegion() string {
	region := strings.ReplaceAll(n.Region, "-", "_")
	return "aws_" + region
}

func (n *NewSnowflakeAccountConfig) LoadPantherAccountAdminRSAKey() (string, error) {
	if _, err := os.Stat(n.PantherAccountAdminRSAKeyOutputPath); os.IsNotExist(err) {
		return "", errors.Errorf(
			"It looks like you haven't created the new Snowflake account yet. The RSA keypair does not exist at location: %s",
			n.PantherAccountAdminRSAKeyOutputPath,
		)
	}

	privateKey, err := os.ReadFile(n.PantherAccountAdminRSAKeyOutputPath)
	if err != nil {
		log.Printf(
			"failed to read PantherAccountAdminRSAKeyOutputPath, error='%s', config section:\n%s",
			err,
			pp.Sprint(n),
		)
		return "", err
	}
	return string(privateKey), nil
}
