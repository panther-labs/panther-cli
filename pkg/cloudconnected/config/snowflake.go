package config

import (
	"crypto/rsa"
	"log"
	"os"
	"strings"

	"github.com/k0kubun/pp/v3"
	"github.com/panther-labs/panther-cli/pkg/rsapem"
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

func (sc *SnowflakeConfig) GetPantherAccountAdminRSAKey() (*rsa.PrivateKey, error) {
	if sc.ConfigType == SnowflakeConfigTypeNewAccount {
		return sc.NewAccountConfig.LoadPantherAccountAdminRSAKey()
	}
	return sc.ExistingAccountConfig.LoadPantherAccountAdminRSAKey()
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
	AccountName                         string          `yaml:"AccountName"                         validate:"required"`
	URL                                 string          `yaml:"URL"                                 validate:"required,url"`
	Edition                             string          `yaml:"Edition"                             validate:"required,oneof=STANDARD ENTERPRISE BUSINESS_CRITICAL"`
	Region                              string          `yaml:"Region"                              validate:"required,lowercase,validPantherRegion"`
	AdminUsername                       string          `yaml:"AdminUsername"                       validate:"required,validAdminName"`
	AdminRSAKeyEnvVarName               string          `yaml:"AdminRSAKeyEnvVarName"               validate:"required_without=AdminRSAKeyPath,excluded_with=AdminRSAKeyPath"`
	AdminRSAKeyPath                     string          `yaml:"AdminRSAKeyPath"                     validate:"required_without=AdminRSAKeyEnvVarName,excluded_with=AdminRSAKeyEnvVarName"`
	PantherAccountAdminRSAKeyOutputPath string          `yaml:"PantherAccountAdminRSAKeyOutputPath" validate:"required"`
	PantherAccountAdminRSAKey           *rsa.PrivateKey `yaml:"-"                                                                                                                         json:"-"`
}

func (e *ExistingSnowflakeAccountConfig) LoadAccountAdminRSAKey() (*rsa.PrivateKey, error) {
	if e.AdminRSAKeyEnvVarName != "" {
		log.Printf(
			"Loading account Admin user ('%s') RSA key from environment variable: %s",
			e.AdminUsername,
			e.AdminRSAKeyEnvVarName,
		)
		privateKeyAsStr := os.Getenv(e.AdminRSAKeyEnvVarName)

		privateKey, err := rsapem.ParseRSAPEMPrivateKey(privateKeyAsStr)
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"failed to decode Panther account admin RSA key from environment variable: %s",
				e.AdminRSAKeyEnvVarName,
			)
		}

		return privateKey, nil
	}

	log.Printf(
		"Loading account Admin user ('%s') RSA key from file: %s",
		e.AdminUsername,
		e.AdminRSAKeyPath,
	)

	privateKeyAsStr, err := os.ReadFile(e.AdminRSAKeyPath)
	if err != nil {
		log.Printf("failed to read PantherAccountAdminRSAKeyPath, error='%s', config section:\n%s", err, pp.Sprint(e))
		return nil, err
	}

	privateKey, err := rsapem.ParseRSAPEMPrivateKey(string(privateKeyAsStr))
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode Panther account admin RSA key")
	}

	return privateKey, nil
}

func (e *ExistingSnowflakeAccountConfig) LoadPantherAccountAdminRSAKey() (*rsa.PrivateKey, error) {
	if e.PantherAccountAdminRSAKey != nil {
		return e.PantherAccountAdminRSAKey, nil
	}

	log.Printf(
		"Loading PANTHERACCOUNTADMIN user RSA key from path: %s", e.PantherAccountAdminRSAKeyOutputPath,
	)

	privateKeyAsStr, err := os.ReadFile(e.AdminRSAKeyPath)
	if err != nil {
		log.Printf("failed to read PantherAccountAdminRSAKeyPath, error='%s', config section:\n%s", err, pp.Sprint(e))
		return nil, err
	}

	privateKey, err := rsapem.ParseRSAPEMPrivateKey(string(privateKeyAsStr))
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode Panther account admin RSA key")
	}

	return privateKey, nil
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

func (n *NewSnowflakeAccountConfig) LoadPantherAccountAdminRSAKey() (*rsa.PrivateKey, error) {
	if _, err := os.Stat(n.PantherAccountAdminRSAKeyOutputPath); os.IsNotExist(err) {
		return nil, errors.Errorf(
			"It looks like you haven't created the new Snowflake account yet. The RSA keypair does not exist at location: %s",
			n.PantherAccountAdminRSAKeyOutputPath,
		)
	}

	privateKeyAsStr, err := os.ReadFile(n.PantherAccountAdminRSAKeyOutputPath)
	if err != nil {
		log.Printf(
			"failed to read PantherAccountAdminRSAKeyOutputPath, error='%s', config section:\n%s",
			err,
			pp.Sprint(n),
		)
		return nil, err
	}

	privateKey, err := rsapem.ParseRSAPEMPrivateKey(string(privateKeyAsStr))
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode Panther account admin RSA key")
	}

	return privateKey, nil
}
