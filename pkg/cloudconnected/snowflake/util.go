package snowflake

import (
	"fmt"
	"os"
	"time"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/snowflakedb/gosnowflake"

	"github.com/cenkalti/backoff/v4"
)

// formatSnowflakeDSNFromSnowflakeOrgConfig generates a DSN string for the Snowflake connection
func formatSnowflakeDSNFromSnowflakeOrgConfig(cfg config.SnowflakeOrgConfig) string {
	return formatSnowflakeDSN(cfg.AccountLocator, cfg.AccountRegion, cfg.OrgAdminUsername, cfg.OrgAdminPassword)
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
