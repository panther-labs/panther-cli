package state

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3" // we do not support any other SQL database
	"github.com/pkg/errors"
)

const (
	dbName         = "panther-cli-state.db"
	createTableSQL = `
	CREATE TABLE IF NOT EXISTS execution_state (
		config_hash TEXT PRIMARY KEY,
		snowflake_admin_username TEXT,
		snowflake_admin_password TEXT,
		snowflake_account_details JSON,
		aws_panther_deployment_role_deployed BOOLEAN,
		aws_readiness_bootstrap_tools_deployed BOOLEAN,
		aws_readiness_check_succeeded BOOLEAN,
		aws_snowflake_bootstrap_succeeded BOOLEAN,
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
		db.Close()
		return nil, errors.Wrap(err, "failed to create table")
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
			snowflake_admin_username,
			snowflake_admin_password,
			snowflake_account_details,
			aws_panther_deployment_role_deployed,
			aws_readiness_bootstrap_tools_deployed,
			aws_readiness_check_succeeded,
			aws_snowflake_bootstrap_succeeded,
			aws_certificates_requested,
			aws_certificates_results
		FROM execution_state
		WHERE config_hash = ?`

	row := &Row{}
	err := d.db.QueryRow(query, configHash).Scan(
		&row.ConfigHash,
		&row.SnowflakeAdminUsername,
		&row.SnowflakeAdminPassword,
		&row.SnowflakeAccountDetails,
		&row.AWSPantherDeploymentRoleDeployed,
		&row.AWSReadinessBootstrapToolsDeployed,
		&row.AWSReadinessCheckSucceeded,
		&row.AWSSnowflakeBootstrapSucceeded,
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

// SaveState saves or updates the execution state
func (d *DB) SaveState(row *Row) error {
	query := `
		INSERT INTO execution_state (
			config_hash,
			snowflake_admin_username,
			snowflake_admin_password,
			snowflake_account_details,
			aws_panther_deployment_role_deployed,
			aws_readiness_bootstrap_tools_deployed,
			aws_readiness_check_succeeded,
			aws_snowflake_bootstrap_succeeded,
			aws_certificates_requested,
			aws_certificates_results
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(config_hash) DO UPDATE SET
			snowflake_admin_username = excluded.snowflake_admin_username,
			snowflake_admin_password = excluded.snowflake_admin_password,
			snowflake_account_details = excluded.snowflake_account_details,
			aws_panther_deployment_role_deployed = excluded.aws_panther_deployment_role_deployed,
			aws_readiness_bootstrap_tools_deployed = excluded.aws_readiness_bootstrap_tools_deployed,
			aws_readiness_check_succeeded = excluded.aws_readiness_check_succeeded,
			aws_snowflake_bootstrap_succeeded = excluded.aws_snowflake_bootstrap_succeeded,
			aws_certificates_requested = excluded.aws_certificates_requested,
			aws_certificates_results = excluded.aws_certificates_results`

	_, err := d.db.Exec(
		query,
		row.ConfigHash,
		row.SnowflakeAdminUsername,
		row.SnowflakeAdminPassword,
		row.SnowflakeAccountDetails,
		row.AWSPantherDeploymentRoleDeployed,
		row.AWSReadinessBootstrapToolsDeployed,
		row.AWSReadinessCheckSucceeded,
		row.AWSSnowflakeBootstrapSucceeded,
		row.AWSCertificatesRequested,
		row.AWSCertificatesResults,
	)
	if err != nil {
		return errors.Wrap(err, "failed to save state")
	}

	return nil
}
