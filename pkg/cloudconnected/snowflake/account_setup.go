package snowflake

import (
	"context"
	"crypto/rsa"
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

func (a *AccountSetup) Connect(ctx context.Context, newAcct *ResolvedSnowflakeAcccount) error {
	tryEnableSnowflakeDebugLogging()

	dsn := util.FormatSnowflakeDSNFromRSAKey(
		newAcct.GetAWSRegion(),
		newAcct.AccountLocator,
		newAcct.AdminUsername,
		"ACCOUNTADMIN",
		newAcct.AdminRSAKey,
	)
	util.LogDebugf("Connecting to Snowflake with DSN: %s", dsn)

	// It can take a little while for a new Snowflake account to
	// be ready to accept connections. We use exponential backoff to wait
	// for the account to be ready.

	oper := func() error {
		log.Println(
			"Attempting to connect to new Snowflake account. This may take a while and you may see this message repeat.",
		)
		db, err := openSnowflakeAccountConnection(ctx, dsn)
		if err != nil {
			return errors.Wrap(err, "failed to open Snowflake account connection")
		}

		a.sql = db
		a.ctx = ctx

		return nil
	}

	if err := backoff.Retry(oper, util.GetDefaultExponentialBackoffRetrier()); err != nil {
		util.LogDebugf(
			"Failed to connect to new Snowflake account (%s) after retries: %v\n",
			newAcct.AccountLocator,
			err,
		)
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

func (a *AccountSetup) CreateCustomerAccountAdminUser(cfg *config.NewSnowflakeAccountConfig) error {
	if !a.isConnected() {
		return errors.New("not connected to Snowflake")
	}

	a.mustSwitchToSecurityAdminRole()

	// create the customer's accountadmin user
	const createUserQuery = `CREATE USER IF NOT EXISTS %s
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

	// result ends up being just a string of the form `User <username> successfully created.`
	var result string
	if err := createUserRow.Scan(&result); err != nil {
		return errors.Wrapf(err, "error scanning result from CREATE USER query")
	}

	expectedCreateUserResults := map[string]string{
		"userCreated": fmt.Sprintf("User %s successfully created.", cfg.AdminUsername),
		"userExists":  fmt.Sprintf("%s already exists, statement succeeded.", cfg.AdminUsername),
	}
	expectedResultFound := false

	for _, expectedResult := range expectedCreateUserResults {
		if strings.EqualFold(result, expectedResult) {
			expectedResultFound = true
			break
		}
	}

	if !expectedResultFound {
		return errors.Errorf("unexpected result when creating %s: %s", cfg.AdminUsername, result)
	}

	if strings.EqualFold(result, expectedCreateUserResults["userCreated"]) {
		log.Printf("Created new Snowflake '%s' user (%s)", cfg.AdminUsername, cfg.AdminEmail)
	} else {
		log.Printf("User %s already exists", cfg.AdminUsername)
	}

	return nil
}

func (a *AccountSetup) CreatePantherAccountAdminUser(
	cfg *config.ExistingSnowflakeAccountConfig,
) (*rsa.PrivateKey, error) {
	if !a.isConnected() {
		return nil, errors.New("not connected to Snowflake")
	}

	a.mustSwitchToSecurityAdminRole()

	// create the customer's accountadmin user
	const createUserQuery = `CREATE USER IF NOT EXISTS %s
  RSA_PUBLIC_KEY = ?
  EMAIL = 'eng-core-infra@runpanther.io'
  TYPE = 'SERVICE'
  DEFAULT_ROLE = 'SYSADMIN'
  MUST_CHANGE_PASSWORD = FALSE;`

	privateKey, pubKeyPEM, err := createAndWritePantherAccountAdminRSAPrivateKey(
		cfg.PantherAccountAdminRSAKeyOutputPath,
	)
	if err != nil {
		return nil, err
	}

	createUserRow := a.conn.QueryRowContext(
		a.ctx,
		fmt.Sprintf(createUserQuery, PantherAccountAdminUserName), // we cannot parameterize the user name
		pubKeyPEM,
	)

	// result ends up being just a string of the form `User PANTHERACCOUNTADMIN successfully created.`
	var result string
	if err := createUserRow.Scan(&result); err != nil {
		return nil, errors.Wrapf(err, "error scanning result from CREATE USER query")
	}

	expectedCreateUserResults := map[string]string{
		"userCreated": fmt.Sprintf("User %s successfully created.", PantherAccountAdminUserName),
		"userExists":  fmt.Sprintf("%s already exists, statement succeeded.", PantherAccountAdminUserName),
	}
	expectedResultFound := false

	for _, expectedResult := range expectedCreateUserResults {
		if strings.EqualFold(result, expectedResult) {
			expectedResultFound = true
			break
		}
	}

	if !expectedResultFound {
		return nil, errors.Errorf("unexpected result when creating %s: %s", PantherAccountAdminUserName, result)
	}

	if strings.EqualFold(result, expectedCreateUserResults["userCreated"]) {
		log.Printf("Created new Snowflake '%s' user", PantherAccountAdminUserName)
	} else {
		log.Printf("User %s already exists", PantherAccountAdminUserName)
	}

	// grant the necessary roles to PANTHERACCOUNTADMIN
	if err := a.GrantPantherAccountAdminUserRoles(); err != nil {
		return nil, errors.Wrap(err, "failed to grant roles to PANTHERACCOUNTADMIN user")
	}

	return privateKey, nil
}

func (a *AccountSetup) GrantPantherAccountAdminUserRoles() error {
	if !a.isConnected() {
		return errors.New("not connected to Snowflake")
	}

	a.mustSwitchToSecurityAdminRole()

	// grant the necessary roles to PANTHERACCOUNTADMIN
	const grantQuery = `GRANT ROLE SYSADMIN, SECURITYADMIN, ACCOUNTADMIN TO USER %s;`

	grantRolesRow := a.conn.QueryRowContext(
		a.ctx,
		fmt.Sprintf(grantQuery, PantherAccountAdminUserName),
	)

	var result string
	if err := grantRolesRow.Scan(&result); err != nil {
		return errors.Wrapf(err, "error scanning result from GRANT ROLE query")
	}

	log.Printf(
		"Granted roles SYSADMIN, SECURITYADMIN, ACCOUNTADMIN to '%s' user: %+v",
		PantherAccountAdminUserName,
		result,
	)

	const alterUserDefaultRoleQuery = `ALTER USER %s SET DEFAULT_ROLE = 'SYSADMIN';`

	alterUserDefaultRoleRow := a.conn.QueryRowContext(
		a.ctx,
		fmt.Sprintf(alterUserDefaultRoleQuery, PantherAccountAdminUserName),
	)

	if err := alterUserDefaultRoleRow.Scan(&result); err != nil {
		return errors.Wrapf(err, "error scanning result from ALTER USER SET DEFAULT_ROLE query")
	}

	log.Printf(
		"Set default role to 'SYSADMIN' for '%s' user: %+v",
		PantherAccountAdminUserName,
		result,
	)

	return nil
}
