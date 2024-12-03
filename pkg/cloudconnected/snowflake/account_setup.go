package snowflake

import (
	"context"
	"database/sql"
	"log"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/util"
	"github.com/pkg/errors"

	"github.com/cenkalti/backoff/v4"
)

// Configuration for Snowflake connection is by using the
// SNOWFLAKE_HOST, SNOWFLAKE_ACCOUNT, SNOWFLAKE_USER, and SNOWFLAKE_PASSWORD
// environment variables for now.
type AccountSetup struct {
	sql *sql.DB
	ctx context.Context
}

func (a *AccountSetup) Connect(ctx context.Context, cfg CreateAccountResult, adminUser string, adminPass string) error {
	tryEnableSnowflakeDebugLogging()

	dsn := formatSnowflakeDSN(cfg.AccountLocator, cfg.GetAWSRegion(), adminUser, adminPass)
	util.LogDebugf("Connecting to Snowflake with DSN: %s", dsn)

	// It can take a little while for a new Snowflake account to
	// be ready to accept connections. We use exponential backoff to wait
	// for the account to be ready.

	oper := func() error {
		log.Println(
			"Attempting to connect to new Snowflake account. This may take a while and you may see this message repeat.",
		)
		db, err := sql.Open("snowflake", dsn)
		if err != nil {
			return errors.Wrap(err, "failed to open connection")
		}

		// Test the connection to make sure it's actually valid.
		if err := db.Ping(); err != nil {
			return errors.Wrap(err, "failed to ping database")
		}

		log.Printf("Successfully connected to new Snowflake account (%s)", cfg.AccountLocator)

		a.sql = db
		a.ctx = ctx

		return nil
	}

	if err := backoff.Retry(oper, getDefaultExponentialBackoffRetrier()); err != nil {
		util.LogDebugf("Failed to connect to new Snowflake account (%s) after retries: %v\n", cfg.AccountLocator, err)
		return errors.Wrap(err, "failed to connect to new Snowflake account")
	}

	return nil
}

func (a *AccountSetup) Close() error {
	return a.sql.Close()
}

func (a *AccountSetup) isConnected() bool {
	return a.sql != nil && a.sql.Ping() == nil
}

func (a *AccountSetup) switchToSecurityAdminRole() error {
	if !a.isConnected() {
		return errors.New("not connected to Snowflake")
	}

	// SQL command to switch to SECURITYADMIN role
	const query = "USE ROLE SECURITYADMIN;"

	// Execute the query
	_, err := a.sql.Exec(query)
	if err != nil {
		return errors.Wrap(err, "failed to switch to SECURITYADMIN role")
	}

	log.Println("Switched to SECURITYADMIN role successfully")
	return nil
}

func (a *AccountSetup) mustSwitchToSecurityAdminRole() {
	if err := a.switchToSecurityAdminRole(); err != nil {
		log.Fatalf("failed to switch to SECURITYADMIN role, you may not have sufficient privileges: %+v\n", err)
	}
}

func (a *AccountSetup) SetupPantherAccountAdminUser(cfg config.PantherAccountAdminConfig) error {
	if !a.isConnected() {
		return errors.New("not connected to Snowflake")
	}

	a.mustSwitchToSecurityAdminRole()

	// create the PANTHERACCOUNTADMIN user
	const createUserQuery = `CREATE USER pantheraccountadmin PASSWORD=? TYPE='LEGACY_SERVICE';`

	createUserRow := a.sql.QueryRowContext(
		a.ctx,
		createUserQuery,
		cfg.PantherAccountAdminPassword,
	)

	// result ends up being just a string of the form `User PANTHERACCOUNTADMIN successfully created.`
	var result string
	if err := createUserRow.Scan(&result); err != nil {
		return errors.Wrapf(err, "error scanning result from CREATE USER query")
	}

	const expectedCreateUserResult = "User PANTHERACCOUNTADMIN successfully created."
	if result != expectedCreateUserResult {
		return errors.Errorf("unexpected result when creating PANTHERACCOUNTADMIN: %s", result)
	}

	log.Println("Created new Snowflake 'pantheraccountadmin' user")

	// grant the necessary roles to PANTHERACCOUNTADMIN
	const grantQuery = `GRANT ROLE SYSADMIN, SECURITYADMIN, ACCOUNTADMIN TO USER pantheraccountadmin;`

	grantRolesRow := a.sql.QueryRowContext(
		a.ctx,
		grantQuery,
	)

	if err := grantRolesRow.Scan(&result); err != nil {
		return errors.Wrapf(err, "error scanning result from GRANT ROLE query")
	}

	log.Printf("Granted roles to 'pantheraccountadmin' user: %+v", result)

	// change PANTHERACCOUNTADMIN's default role to SYSADMIN
	const alterDefaultRoleQuery = `ALTER USER pantheraccountadmin SET DEFAULT_ROLE = SYSADMIN;`

	alterDefaultRoleRow := a.sql.QueryRowContext(
		a.ctx,
		alterDefaultRoleQuery,
	)

	if err := alterDefaultRoleRow.Scan(&result); err != nil {
		return errors.Wrapf(err, "error scanning result from ALTER USER query")
	}

	log.Printf("Altered default role for 'pantheraccountadmin' user: %+v", result)

	return nil
}
