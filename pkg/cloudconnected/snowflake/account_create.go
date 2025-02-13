package snowflake

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"github.com/pkg/errors"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/rsapem"
	"github.com/panther-labs/panther-cli/pkg/util"
)

// Configuration for Snowflake connection is by using the
// SNOWFLAKE_HOST, SNOWFLAKE_ACCOUNT, SNOWFLAKE_USER, and SNOWFLAKE_PASSWORD
// environment variables for now.
type AccountCreate struct {
	sql *sql.DB
	ctx context.Context
}

func (a *AccountCreate) Connect(ctx context.Context, cfg config.SnowflakeOrgConfig) error {
	tryEnableSnowflakeDebugLogging()

	dsn := formatSnowflakeDSNFromSnowflakeOrgConfig(cfg)
	util.LogDebugf("Connecting to Snowflake with DSN: %s", dsn)

	db, err := sql.Open("snowflake", dsn)
	if err != nil {
		return errors.Wrap(err, "failed to open connection")
	}

	// Test the connection to make sure it's actually valid.
	if err := db.Ping(); err != nil {
		return errors.Wrap(err, "failed to ping database")
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

// New accounts are created with PANTHERACCOUNTADMIN and a newly generated RSA key
func (a *AccountCreate) CreateNewSnowflakeAccount(cfg config.NewAccountConfig) (CreateAccountResult, error) {
	if !a.isConnected() {
		return CreateAccountResult{}, errors.New("not connected to Snowflake")
	}

	a.mustSwitchToOrgAdminRole()

	var createAcctRes CreateAccountResult

	key, err := rsapem.GenerateKeyPair()
	if err != nil {
		return CreateAccountResult{}, errors.Wrap(err, "failed to generate PANTHERACCOUNTADMIN RSA key pair")
	}
	pubkey, err := rsapem.EncodeRSAPEMPublicKey(key.PublicKey)
	if err != nil {
		return CreateAccountResult{}, errors.Wrap(err, "failed to encode PANTHERACCOUNTADMIN RSA public key")
	}
	createAcctRes.AdminRSAKey = key.PrivateKey

	const query = `
CREATE ACCOUNT %s
  ADMIN_NAME = 'PANTHERACCOUNTADMIN'
  ADMIN_RSA_PUBLIC_KEY = ?
  ADMIN_USER_TYPE = 'SERVICE'
  MUST_CHANGE_PASSWORD = FALSE
  EDITION = ?
  REGION = ?
  COMMENT = 'Panther Snowflake Cloud Connected Production Environment';
	`

	row := a.sql.QueryRowContext(
		a.ctx,
		fmt.Sprintf(query, cfg.AccountName), // we cannot parameterize the account name
		pubkey,
		cfg.SnowflakeEdition,
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

	log.Printf("Created new Snowflake account: %+v", createAcctRes)

	return createAcctRes, nil
}

func (a *AccountCreate) Close() error {
	return a.sql.Close()
}
