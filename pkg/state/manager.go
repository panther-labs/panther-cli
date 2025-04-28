package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/aws"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/snowflake"
)

// Manager handles state operations for the setup process
type Manager struct {
	db         *DB
	configHash string
	state      *Row
}

// NewManager creates a new state manager for a given config
func NewManager(cfg *config.Config) (*Manager, error) {
	// Calculate config hash
	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal config")
	}
	hash := sha256.Sum256(cfgBytes)
	configHash := hex.EncodeToString(hash[:])

	// Initialize database
	db, err := NewDB()
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize database")
	}

	// Get existing state or create new one
	state, err := db.GetState(configHash)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get state")
	}
	if state == nil {
		state = &Row{
			ConfigHash: configHash,
		}
	}

	return &Manager{
		db:         db,
		configHash: configHash,
		state:      state,
	}, nil
}

// Close closes the database connection
func (m *Manager) Close() error {
	return m.db.Close()
}

// SaveState saves the current state to the database
func (m *Manager) SaveState() error {
	return m.db.SaveState(m.state)
}

// GetState returns the current state
func (m *Manager) GetState() *Row {
	return m.state
}

// UpdateSnowflakeState updates the Snowflake-related state
func (m *Manager) UpdateSnowflakeState(accountDetails *snowflake.ResolvedSnowflakeAcccount, accountSetup bool) error {
	m.state.PopulateSnowflakeAccountDetails(accountDetails)
	m.state.SnowflakeAccountSetup = accountSetup
	return m.SaveState()
}

// UpdateAWSDeploymentState updates the AWS deployment role state
func (m *Manager) UpdateAWSDeploymentState(deployed bool) error {
	m.state.AWSPantherDeploymentRoleDeployed = deployed
	return m.SaveState()
}

// UpdateAWSBootstrapState updates the AWS bootstrap tools state
func (m *Manager) UpdateAWSBootstrapState(deployed bool) error {
	m.state.AWSReadinessBootstrapToolsDeployed = deployed
	return m.SaveState()
}

// UpdateAWSReadinessState updates only the AWS readiness check state and results
func (m *Manager) UpdateAWSReadinessState(results ReadinessCheckResults) error {
	m.state.AWSReadinessCheckResults = results
	m.state.AWSReadinessCheckSucceeded = results.HasPassed()
	return m.SaveState()
}

// UpdateAWSSnowflakeBootstrapState updates only the AWS Snowflake bootstrap state
func (m *Manager) UpdateAWSSnowflakeBootstrapState(succeeded bool, secretARN string) error {
	m.state.AWSSnowflakeBootstrapSucceeded = succeeded
	m.state.AWSSnowflakeSecretARN = secretARN
	return m.SaveState()
}

// UpdateCertificateState updates the certificate state with new results
func (m *Manager) UpdateCertificateState(
	certType string,
	result aws.CertificateRegistrationResult,
	isIssued bool,
) error {
	if !m.state.AWSCertificatesRequested {
		m.state.AWSCertificatesRequested = true
	}

	certRecord := &CertificateRecord{
		CertificateArn: result.CertificateArn,
		ValidationDetails: CertificateValidationRecord{
			DomainName:  result.ValidationDetails.DomainName,
			RecordName:  result.ValidationDetails.RecordName,
			RecordValue: result.ValidationDetails.RecordValue,
			RecordType:  result.ValidationDetails.RecordType,
		},
		IsIssued: isIssued,
	}

	switch certType {
	case "panther":
		m.state.AWSCertificatesResults.PantherSubdomain = certRecord
	case "wildcard":
		m.state.AWSCertificatesResults.WildcardSubdomain = certRecord
	default:
		return errors.Errorf("invalid certificate type: %s", certType)
	}

	return m.SaveState()
}
