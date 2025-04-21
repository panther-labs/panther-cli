package snowflake

import (
	"context"
	"database/sql"
	"log"
	"os"

	"github.com/pkg/errors"
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

func openSnowflakeAccountConnection(ctx context.Context, dsn string) (*sql.DB, error) {
	tryEnableSnowflakeDebugLogging()

	util.LogDebugf("Connecting to Snowflake with DSN: %s", dsn)

	db, err := sql.Open("snowflake", dsn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open connection")
	}

	// Test the connection to make sure it's actually valid.
	if err := db.Ping(); err != nil {
		return nil, errors.Wrap(err, "failed to ping database")
	}

	util.LogDebugf("Successfully connected to Snowflake account (%s)", dsn)
	return db, nil
}

func OpenSnowflakeAccountConnection(ctx context.Context, dsn string) (*sql.DB, error) {
	return openSnowflakeAccountConnection(ctx, dsn)
}
