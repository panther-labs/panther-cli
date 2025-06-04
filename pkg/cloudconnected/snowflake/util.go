package snowflake

import (
	"context"
	"crypto/rsa"
	"database/sql"
	"fmt"
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
	config, err := util.NewSnowflakeConnectionConfigWithLocatorAndRegion(
		cfg.AccountRegion,
		cfg.AccountLocator,
		cfg.OrgAdminUsername,
		util.SnowflakeRoleOrgAdmin,
		parsedPrivateKey,
	)
	if err != nil {
		log.Fatalf("failed to create Snowflake connection config: %s", err.Error())
	}

	dsn, err := config.GetDSN()
	if err != nil {
		log.Fatalf("failed to generate Snowflake DSN for credentials: %s", err.Error())
	}
	return dsn
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
	if err := db.PingContext(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to ping database")
	}

	util.LogDebugf("Successfully connected to Snowflake account (%s)", dsn)
	return db, nil
}

func OpenSnowflakeAccountConnection(ctx context.Context, dsn string) (*sql.DB, error) {
	return openSnowflakeAccountConnection(ctx, dsn)
}

// createAndWritePantherAccountAdminRSAPrivateKey generates a new RSA key pair,
// writes the private key (PEM encoded) to the path specified in cfg.PantherAccountAdminRSAKeyOutputPath,
// and returns the private key object and the PEM encoded public key string.
func createAndWritePantherAccountAdminRSAPrivateKey(
	pantherAccountAdminRSAKeyOutputPath string,
) (*rsa.PrivateKey, string, error) {
	key, err := rsapem.GenerateKeyPair()
	if err != nil {
		return nil, "", errors.Wrap(err, fmt.Sprintf("failed to generate %s RSA key pair", PantherAccountAdminUserName))
	}

	pubkeyPEM, err := rsapem.EncodeRSAPEMPublicKey(key.PublicKey)
	if err != nil {
		return nil, "", errors.Wrap(err, fmt.Sprintf("failed to encode %s RSA public key", PantherAccountAdminUserName))
	}

	// Encode and write the private key to the specified output path
	privKeyPEMBytes, err := rsapem.EncodeRSAPEMPrivateKey(key.PrivateKey)
	if err != nil {
		return nil, "", errors.Wrapf(
			err,
			"failed to encode %s RSA private key",
			PantherAccountAdminUserName,
		)
	}
	// Ensure the key is written with restricted permissions (owner read/write only)
	err = os.WriteFile(pantherAccountAdminRSAKeyOutputPath, []byte(privKeyPEMBytes), 0o600)
	if err != nil {
		return nil, "", errors.Wrapf(
			err,
			"failed to write %s RSA private key to %s",
			PantherAccountAdminUserName,
			pantherAccountAdminRSAKeyOutputPath,
		)
	}
	log.Printf("Wrote %s RSA private key to %s", PantherAccountAdminUserName, pantherAccountAdminRSAKeyOutputPath)

	return key.PrivateKey, pubkeyPEM, nil
}
