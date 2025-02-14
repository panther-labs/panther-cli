package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/pkg/errors"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/util"

	"github.com/cenkalti/backoff/v4"
)

type AccountSetup struct {
	sql  *sql.DB
	conn *sql.Conn
	ctx  context.Context
}

func (a *AccountSetup) Connect(ctx context.Context, cfg CreateAccountResult) error {
	tryEnableSnowflakeDebugLogging()

	dsn := formatSnowflakeDSNFromRSAKey(cfg.GetAWSRegion(), cfg.AccountLocator, "PANTHERACCOUNTADMIN", "ACCOUNTADMIN", cfg.AdminRSAKey)
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

// creates a conn and sets the role; needed to guarantee the role is used on subsequent queries
func (a *AccountSetup) switchToSecurityAdminRole() error {
	if !a.isConnected() {
		return errors.New("not connected to Snowflake")
	}

	var err error
	a.conn, err = a.sql.Conn(context.Background())
	if err != nil {
		return errors.Wrap(err, "failed to get connection from pool")
	}

	// SQL command to switch to SECURITYADMIN role
	const query = "USE ROLE SECURITYADMIN;"

	// Execute the query
	_, err = a.conn.ExecContext(a.ctx, query)
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

func (a *AccountSetup) SetupCustomerAccountAdminUser(cfg config.NewAccountConfig) error {
	if !a.isConnected() {
		return errors.New("not connected to Snowflake")
	}

	a.mustSwitchToSecurityAdminRole()

	// create the customer's accountadmin user
	const createUserQuery = `CREATE USER %s
  PASSWORD = ?
  EMAIL = ?
  TYPE = 'PERSON'
  DEFAULT_ROLE = 'SYSADMIN'
  MUST_CHANGE_PASSWORD = FALSE;`

	createUserRow := a.conn.QueryRowContext(
		a.ctx,
		fmt.Sprintf(createUserQuery, cfg.AdminUsername), // we cannot parameterize the user name
		cfg.AdminPassword,
		cfg.AdminEmail,
	)

	// result ends up being just a string of the form `User PANTHERACCOUNTADMIN successfully created.`
	var result string
	if err := createUserRow.Scan(&result); err != nil {
		return errors.Wrapf(err, "error scanning result from CREATE USER query")
	}

	expectedCreateUserResult := fmt.Sprintf("User %s successfully created.", cfg.AdminUsername)
	if !strings.EqualFold(result, expectedCreateUserResult) {
		return errors.Errorf("unexpected result when creating %s: %s", cfg.AdminUsername, result)
	}

	log.Printf("Created new Snowflake '%s' user (%s)", cfg.AdminUsername, cfg.AdminEmail)

	// grant the necessary roles to PANTHERACCOUNTADMIN
	const grantQuery = `GRANT ROLE SYSADMIN, SECURITYADMIN, ACCOUNTADMIN TO USER %s;`

	grantRolesRow := a.conn.QueryRowContext(
		a.ctx,
		fmt.Sprintf(grantQuery, cfg.AdminUsername),
	)

	if err := grantRolesRow.Scan(&result); err != nil {
		return errors.Wrapf(err, "error scanning result from GRANT ROLE query")
	}

	log.Printf("Granted roles SYSADMIN, SECURITYADMIN, ACCOUNTADMIN to '%s' user: %+v", cfg.AdminUsername, result)

	return nil
}
