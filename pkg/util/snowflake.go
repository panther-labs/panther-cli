package util

import (
	"crypto/rsa"
	"fmt"
	"log"

	"github.com/k0kubun/pp/v3"
	"github.com/panther-labs/panther-cli/pkg/rsapem"
	"github.com/snowflakedb/gosnowflake"
)

// FormatSnowflakeDSNFromRSAKey uses gosnowflake to generate a JWT-based connection string
func FormatSnowflakeDSNFromRSAKey(region, locator, username, role string, key *rsa.PrivateKey) string {
	const hostFormat = "%s.%s.snowflakecomputing.com"
	host := fmt.Sprintf(hostFormat, locator, region)

	log.Printf(
		"Connecting to Snowflake using private key with public key:\n%v",
		rsapem.MustFormatPublicKey(key),
	)

	connConfig := &gosnowflake.Config{
		Account:       locator,
		Authenticator: gosnowflake.AuthTypeJwt,
		User:          username,
		PrivateKey:    key,
		Role:          role,
	}

	// If region is provided, use it to set the region in the connection config.
	if region != "" {
		connConfig.Region = region
		connConfig.Host = host
	}

	dsn, err := gosnowflake.DSN(connConfig)
	if err != nil {
		log.Fatalf(
			"failed to generate Snowflake DSN for credentials: %s\ngosnowflake.Config: %s\narguments(region=%s, locator=%s, username=%s, role=%s)",
			err.Error(),
			pp.Sprint(connConfig),
			region,
			locator,
			username,
			role,
		)
	}

	return dsn
}
