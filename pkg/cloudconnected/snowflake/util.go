package snowflake

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/util"
	"github.com/snowflakedb/gosnowflake"

	"github.com/cenkalti/backoff/v4"
)

// formatSnowflakeDSNFromSnowflakeOrgConfig generates a DSN string for the Snowflake connection
func formatSnowflakeDSNFromSnowflakeOrgConfig(cfg config.SnowflakeOrgConfig) string {
	const hostFormat = "%s.%s.snowflakecomputing.com"
	host := fmt.Sprintf(hostFormat, cfg.AccountLocator, cfg.AccountRegion)

	parsedPrivateKey, err := util.ParseRSAPEMPrivateKey(cfg.OrgAdminPrivateKey)
	if err != nil {
		log.Fatalf("failed to parse RSA private key for Snowflake ORGADMIN credentials: %s", err.Error())
	}

	log.Printf(
		"Connecting to Snowflake using private key with public key:\n%v",
		util.MustFormatPublicKeyFromPrivateKey(parsedPrivateKey),
	)

	connConfig := &gosnowflake.Config{
		Account:       cfg.AccountLocator,
		Region:        cfg.AccountRegion,
		Host:          host,
		Authenticator: gosnowflake.AuthTypeJwt,
		User:          cfg.OrgAdminUsername,
		PrivateKey:    parsedPrivateKey,
		Role:          "ORGADMIN",
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
