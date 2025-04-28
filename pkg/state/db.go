package state

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3" // we do not support any other SQL database
	"github.com/pkg/errors"
)

const (
	dbName         = "panther-cli-state.db"
	createTableSQL = `
	CREATE TABLE IF NOT EXISTS execution_state (
		config_hash TEXT PRIMARY KEY,
		snowflake_account_name TEXT,
		snowflake_account_url TEXT,
		snowflake_edition TEXT,
		snowflake_region TEXT,
		snowflake_account_setup BOOLEAN,
		aws_panther_deployment_role_deployed BOOLEAN,
		aws_readiness_bootstrap_tools_deployed BOOLEAN,
		aws_readiness_check_succeeded BOOLEAN,
		aws_readiness_check_results JSON,
		aws_snowflake_bootstrap_succeeded BOOLEAN,
		aws_snowflake_secret_arn TEXT,
		aws_certificates_requested BOOLEAN,
		aws_certificates_results JSON
	)`
)

type DB struct {
	db *sql.DB
}

// NewDB creates a new database connection in the current directory
func NewDB() (*DB, error) {
	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open database")
	}

	if _, err := db.Exec(createTableSQL); err != nil {
		if err := db.Close(); err != nil {
			return nil, errors.Wrap(err, "failed to create table")
		}
		return nil, errors.Wrap(err, "failed to create table")
	}

	// Ensure durability by:
	// - setting synchronous mode to EXTRA
	// - setting journal mode to DELETE
	//
	// We don't care about performance here, we just want to ensure that the
	// database is durable and that we can recover from any crash. Writes are
	// very infrequent.
	if _, err := db.Exec("PRAGMA synchronous=EXTRA; PRAGMA journal_mode = DELETE;"); err != nil {
		if err := db.Close(); err != nil {
			return nil, errors.Wrap(err, "failed to set synchronous mode")
		}
		return nil, errors.Wrap(err, "failed to set synchronous mode")
	}

	return &DB{db: db}, nil
}

// Close closes the database connection
func (d *DB) Close() error {
	return d.db.Close()
}

// GetState retrieves the execution state for a given config hash
func (d *DB) GetState(configHash string) (*Row, error) {
	query := `
		SELECT
			config_hash,
			snowflake_account_name,
			snowflake_account_url,
			snowflake_edition,
			snowflake_region,
			snowflake_account_setup,
			aws_panther_deployment_role_deployed,
			aws_readiness_bootstrap_tools_deployed,
			aws_readiness_check_succeeded,
			aws_readiness_check_results,
			aws_snowflake_bootstrap_succeeded,
			aws_snowflake_secret_arn,
			aws_certificates_requested,
			aws_certificates_results
		FROM execution_state
		WHERE config_hash = ?`

	row := &Row{}
	err := d.db.QueryRow(query, configHash).Scan(
		&row.ConfigHash,
		&row.SnowflakeAccountName,
		&row.SnowflakeAccountURL,
		&row.SnowflakeEdition,
		&row.SnowflakeRegion,
		&row.SnowflakeAccountSetup,
		&row.AWSPantherDeploymentRoleDeployed,
		&row.AWSReadinessBootstrapToolsDeployed,
		&row.AWSReadinessCheckSucceeded,
		&row.AWSReadinessCheckResults,
		&row.AWSSnowflakeBootstrapSucceeded,
		&row.AWSSnowflakeSecretARN,
		&row.AWSCertificatesRequested,
		&row.AWSCertificatesResults,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to get state")
	}

	return row, nil
}

// HasState checks if the state database file exists and has any state
func HasState() bool {
	if _, err := os.Stat(dbName); os.IsNotExist(err) {
		return false
	}

	// Open database to check if it has any rows
	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		return false
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Fatalf("failed to close database: %v\n", err)
		}
	}()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM execution_state").Scan(&count)
	if err != nil || count == 0 {
		return false
	}

	return true
}

// CleanState removes the state database file
func CleanState() error {
	if err := os.Remove(dbName); err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err, "failed to remove state database")
		}
		return nil
	}
	return nil
}

// SaveState saves or updates the execution state
func (d *DB) SaveState(row *Row) error {
	query := `
		INSERT INTO execution_state (
			config_hash,
			snowflake_account_name,
			snowflake_account_url,
			snowflake_edition,
			snowflake_region,
			snowflake_account_setup,
			aws_panther_deployment_role_deployed,
			aws_readiness_bootstrap_tools_deployed,
			aws_readiness_check_succeeded,
			aws_readiness_check_results,
			aws_snowflake_bootstrap_succeeded,
			aws_snowflake_secret_arn,
			aws_certificates_requested,
			aws_certificates_results
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(config_hash) DO UPDATE SET
			snowflake_account_name = excluded.snowflake_account_name,
			snowflake_account_url = excluded.snowflake_account_url,
			snowflake_edition = excluded.snowflake_edition,
			snowflake_account_setup = excluded.snowflake_account_setup,
			aws_panther_deployment_role_deployed = excluded.aws_panther_deployment_role_deployed,
			aws_readiness_bootstrap_tools_deployed = excluded.aws_readiness_bootstrap_tools_deployed,
			aws_readiness_check_succeeded = excluded.aws_readiness_check_succeeded,
			aws_readiness_check_results = excluded.aws_readiness_check_results,
			aws_snowflake_bootstrap_succeeded = excluded.aws_snowflake_bootstrap_succeeded,
			aws_snowflake_secret_arn = excluded.aws_snowflake_secret_arn,
			aws_certificates_requested = excluded.aws_certificates_requested,
			aws_certificates_results = excluded.aws_certificates_results`

	_, err := d.db.Exec(
		query,
		row.ConfigHash,
		row.SnowflakeAccountName,
		row.SnowflakeAccountURL,
		row.SnowflakeEdition,
		row.SnowflakeRegion,
		row.SnowflakeAccountSetup,
		row.AWSPantherDeploymentRoleDeployed,
		row.AWSReadinessBootstrapToolsDeployed,
		row.AWSReadinessCheckSucceeded,
		row.AWSReadinessCheckResults,
		row.AWSSnowflakeBootstrapSucceeded,
		row.AWSSnowflakeSecretARN,
		row.AWSCertificatesRequested,
		row.AWSCertificatesResults,
	)
	if err != nil {
		return errors.Wrap(err, "failed to save state")
	}

	return nil
}
