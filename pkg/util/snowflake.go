package util

import (
	"crypto/rsa"
	"fmt"
	"log"

	"github.com/k0kubun/pp/v3"
	"github.com/panther-labs/panther-cli/pkg/rsapem"
	"github.com/pkg/errors"
	"github.com/snowflakedb/gosnowflake"
)

const (
	SnowflakeRoleAccountAdmin  = "ACCOUNTADMIN"
	SnowflakeRoleSecurityAdmin = "SECURITYADMIN"
	SnowflakeRoleSysAdmin      = "SYSADMIN"
	SnowflakeRoleOrgAdmin      = "ORGADMIN"
)

// SnowflakeConnectionConfig represents the configuration needed to connect to Snowflake
type SnowflakeConnectionConfig struct {
	region      string
	locator     string
	accountName string
	username    string
	role        string
	privateKey  *rsa.PrivateKey
}

func NewSnowflakeConnectionConfigWithAccountName(
	acctName string,
	username string,
	role string,
	key *rsa.PrivateKey,
) (*SnowflakeConnectionConfig, error) {
	if acctName == "" {
		return nil, errors.New("account name is required to generate a valid Snowflake connection config")
	}

	if username == "" {
		return nil, errors.New("username is required to generate a valid Snowflake connection config")
	}

	if role == "" {
		return nil, errors.New("role is required to generate a valid Snowflake connection config")
	}

	if key == nil {
		return nil, errors.New("private key is required to generate a valid Snowflake connection config")
	}

	return &SnowflakeConnectionConfig{
		region:      "",
		locator:     "",
		accountName: acctName,
		username:    username,
		role:        role,
		privateKey:  key,
	}, nil
}

func NewSnowflakeConnectionConfigWithLocatorAndRegion(
	region string,
	locator string,
	username string,
	role string,
	key *rsa.PrivateKey,
) (*SnowflakeConnectionConfig, error) {
	if locator == "" {
		return nil, errors.New("locator is required to generate a valid Snowflake connection config")
	}

	if region == "" {
		return nil, errors.New(
			"region is required to generate a valid Snowflake connection config when locator is provided",
		)
	}

	if username == "" {
		return nil, errors.New("username is required to generate a valid Snowflake connection config")
	}

	if role == "" {
		return nil, errors.New("role is required to generate a valid Snowflake connection config")
	}

	if key == nil {
		return nil, errors.New("private key is required to generate a valid Snowflake connection config")
	}

	return &SnowflakeConnectionConfig{
		region:      region,
		locator:     locator,
		accountName: "",
		username:    username,
		role:        role,
		privateKey:  key,
	}, nil
}

// GetDSN generates a JWT-based connection string for Snowflake using the configuration
func (c *SnowflakeConnectionConfig) GetDSN() (string, error) {
	log.Printf(
		"Connecting to Snowflake using private key with public key:\n%v",
		rsapem.MustFormatPublicKey(c.privateKey),
	)

	log.Printf(
		"Generating Snowflake DSN for credentials: username=%s, accountLocator=%s, accountName=%s, role=%s\n",
		c.username,
		c.locator,
		c.accountName,
		c.role,
	)

	connConfig := &gosnowflake.Config{
		Authenticator: gosnowflake.AuthTypeJwt,
		User:          c.username,
		PrivateKey:    c.privateKey,
		Role:          c.role,
	}

	// Account names are global. However, if we need to construct a DSN using the locator, then we also need a region.
	if c.region != "" && c.locator != "" {
		const hostFormat = "%s.%s.snowflakecomputing.com"
		host := fmt.Sprintf(hostFormat, c.locator, c.region)
		connConfig.Host = host
		connConfig.Account = c.locator
	} else if c.accountName != "" {
		connConfig.Account = c.accountName
	}

	dsn, err := gosnowflake.DSN(connConfig)
	if err != nil {
		return "", errors.Wrapf(
			err,
			"failed to generate Snowflake DSN for credentials: gosnowflake.Config: %s",
			pp.Sprint(c),
		)
	}

	return dsn, nil
}
