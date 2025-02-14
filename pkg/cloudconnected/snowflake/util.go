package snowflake

import (
	"crypto/rsa"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/snowflakedb/gosnowflake"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/rsapem"

	"github.com/cenkalti/backoff/v4"
)

// formatSnowflakeDSNFromSnowflakeOrgConfig generates a DSN string for the Snowflake connection
func formatSnowflakeDSNFromSnowflakeOrgConfig(cfg config.SnowflakeOrgConfig) string {
	parsedPrivateKey, err := rsapem.ParseRSAPEMPrivateKey(cfg.OrgAdminPrivateKey)
	if err != nil {
		log.Fatalf("failed to parse RSA private key for Snowflake ORGADMIN credentials: %s", err.Error())
	}
	return formatSnowflakeDSNFromRSAKey(cfg.AccountRegion, cfg.AccountLocator, cfg.OrgAdminUsername, "ORGADMIN", parsedPrivateKey)
}

// formatSnowflakeDSNFromRSAKey uses gosnowflake to generate a JWT-based connection string
func formatSnowflakeDSNFromRSAKey(region, locator, username, role string, key *rsa.PrivateKey) string {
	const hostFormat = "%s.%s.snowflakecomputing.com"
	host := fmt.Sprintf(hostFormat, locator, region)

	log.Printf(
		"Connecting to Snowflake using private key with public key:\n%v",
		rsapem.MustFormatPublicKey(key),
	)

	connConfig := &gosnowflake.Config{
		Account:       locator,
		Region:        region,
		Host:          host,
		Authenticator: gosnowflake.AuthTypeJwt,
		User:          username,
		PrivateKey:    key,
		Role:          role,
	}

	dsn, err := gosnowflake.DSN(connConfig)
	if err != nil {
		log.Fatalf("failed to generate Snowflake DSN for ORGADMIN credentials: %s", err.Error())
	}

	return dsn
}

func formatSnowflakeDSN(accountLocator, accountRegion, username, password string) string {
	const dsnFormat = "%s:%s@%s.%s.snowflakecomputing.com"

	return fmt.Sprintf(
		dsnFormat,
		username,
		password,
		accountLocator,
		accountRegion,
	)
}

func getDefaultExponentialBackoffRetrier() *backoff.ExponentialBackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 5 * time.Second
	b.MaxInterval = 30 * time.Second
	b.MaxElapsedTime = 5 * time.Minute
	return b
}

func tryEnableSnowflakeDebugLogging() {
	if os.Getenv("SNOWFLAKE_DEBUG") != "" {
		_ = gosnowflake.GetLogger().SetLogLevel("debug")
	}
}
