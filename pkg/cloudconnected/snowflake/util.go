package snowflake

import (
	"log"
	"os"

	"github.com/snowflakedb/gosnowflake"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/rsapem"
	"github.com/panther-labs/panther-cli/pkg/util"
)

// formatSnowflakeDSNFromSnowflakeOrgConfig generates a DSN string for the Snowflake connection
func formatSnowflakeDSNFromSnowflakeOrgConfig(cfg config.SnowflakeOrgConfig) string {
	parsedPrivateKey, err := rsapem.ParseRSAPEMPrivateKey(cfg.OrgAdminPrivateKey)
	if err != nil {
		log.Fatalf("failed to parse RSA private key for Snowflake ORGADMIN credentials: %s", err.Error())
	}
	return util.FormatSnowflakeDSNFromRSAKey(
		cfg.AccountRegion,
		cfg.AccountLocator,
		cfg.OrgAdminUsername,
		"ORGADMIN",
		parsedPrivateKey,
	)
}

func tryEnableSnowflakeDebugLogging() {
	if os.Getenv("SNOWFLAKE_DEBUG") != "" {
		_ = gosnowflake.GetLogger().SetLogLevel("debug")
	}
}
