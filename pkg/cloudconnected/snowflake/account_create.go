package snowflake

import (
	"context"
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/k0kubun/pp/v3"
	"github.com/pkg/errors"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/rsapem"
)

// Configuration for Snowflake connection is by using the
// SNOWFLAKE_HOST, SNOWFLAKE_ACCOUNT, SNOWFLAKE_USER, and SNOWFLAKE_PASSWORD
// environment variables for now.
type AccountCreate struct {
	sql *sql.DB
	ctx context.Context
}

func (a *AccountCreate) Connect(ctx context.Context, cfg config.SnowflakeOrgConfig) error {
	dsn := formatSnowflakeDSNFromSnowflakeOrgConfig(cfg)

	db, err := openSnowflakeAccountConnection(ctx, dsn)
	if err != nil {
		return errors.Wrap(err, "failed to open Snowflake account connection")
	}

	a.sql = db
	a.ctx = ctx

	return nil
}

func (a *AccountCreate) isConnected() bool {
	return a.sql != nil && a.sql.Ping() == nil
}

func (a *AccountCreate) switchToOrgAdminRole() error {
	if !a.isConnected() {
		return errors.New("not connected to Snowflake")
	}

	// SQL command to switch to ORGADMIN role
	const query = "USE ROLE ORGADMIN;"

	// Execute the query
	_, err := a.sql.Exec(query)
	if err != nil {
		return errors.Wrap(err, "failed to switch to ORGADMIN role")
	}

	log.Println("Switched to ORGADMIN role successfully")
	return nil
}

func (a *AccountCreate) mustSwitchToOrgAdminRole() {
	if err := a.switchToOrgAdminRole(); err != nil {
		log.Fatalf("failed to switch to ORGADMIN role, you may not have sufficient privileges: %+v\n", err)
	}
}

// CreateAndWritePantherAccountAdminRSAKeyPair generates a new RSA key pair,
// writes the private key (PEM encoded) to the path specified in cfg.PantherAccountAdminRSAKeyOutputPath,
// and returns the private key object and the PEM encoded public key string.
func CreateAndWritePantherAccountAdminRSAKeyPair(
	cfg *config.NewSnowflakeAccountConfig,
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
	err = os.WriteFile(cfg.PantherAccountAdminRSAKeyOutputPath, []byte(privKeyPEMBytes), 0o600)
	if err != nil {
		return nil, "", errors.Wrapf(
			err,
			"failed to write %s RSA private key to %s",
			PantherAccountAdminUserName,
			cfg.PantherAccountAdminRSAKeyOutputPath,
		)
	}
	log.Printf("Wrote %s RSA private key to %s", PantherAccountAdminUserName, cfg.PantherAccountAdminRSAKeyOutputPath)

	return key.PrivateKey, pubkeyPEM, nil
}

// New accounts are created with PANTHERACCOUNTADMIN and a newly generated RSA key
func (a *AccountCreate) CreateNewSnowflakeAccount(
	cfg *config.NewSnowflakeAccountConfig,
) (*ResolvedSnowflakeAcccount, error) {
	if !a.isConnected() {
		return nil, errors.New("not connected to Snowflake")
	}

	a.mustSwitchToOrgAdminRole()

	createAcctRes := &ResolvedSnowflakeAcccount{}

	privateKey, pubkeyPEM, err := CreateAndWritePantherAccountAdminRSAKeyPair(cfg)
	if err != nil {
		return nil, err
	}

	const query = `
CREATE ACCOUNT %s
  ADMIN_NAME = ?
  ADMIN_RSA_PUBLIC_KEY = ?
  ADMIN_USER_TYPE = 'SERVICE'
  MUST_CHANGE_PASSWORD = FALSE
  EMAIL = 'eng-core-infra@runpanther.io'
  EDITION = ?
  REGION = ?
  COMMENT = 'Panther Snowflake Cloud Connected Production Environment';
	`

	row := a.sql.QueryRowContext(
		a.ctx,
		fmt.Sprintf(query, cfg.AccountName), // we cannot parameterize the account name
		PantherAccountAdminUserName,
		pubkeyPEM,
		cfg.Edition,
		cfg.GetSnowflakeRegion(),
	)

	var result string
	if err := row.Scan(&result); err != nil {
		return createAcctRes, errors.Wrapf(err, "error scanning result from CREATE ACCOUNT query")
	}

	err = json.Unmarshal(json.RawMessage(result), &createAcctRes)
	if err != nil {
		return createAcctRes, errors.Wrap(err, "error unmarshalling of CREATE ACCOUNT output")
	}

	// Snowflake is weird and when you create an account, it returns the account name
	// without the parent organization prefix. That prefix lives in the `URL` field in the
	// response payload.
	fullyQualifiedAccountName, err := createAcctRes.GetFullyQualifiedAccountName()
	if err != nil {
		return createAcctRes, errors.Wrap(err, "error getting fully qualified account name")
	}
	createAcctRes.AccountName = fullyQualifiedAccountName

	log.Printf("Created new Snowflake account: %v", pp.Sprintln(createAcctRes))
	// log before adding the RSA key to the result
	createAcctRes.AdminRSAKey = privateKey

	return createAcctRes, nil
}

func (a *AccountCreate) Close() error {
	return a.sql.Close()
}
