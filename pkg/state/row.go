package state

import (
	"database/sql/driver"
	"encoding/json"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/snowflake"
	"github.com/pkg/errors"
)

// SnowflakeAccountDetails wraps snowflake.CreateAccountResult for JSON serialization
type SnowflakeAccountDetails struct {
	snowflake.CreateAccountResult
}

// Value implements the driver.Valuer interface for JSON storage
func (s SnowflakeAccountDetails) Value() (driver.Value, error) {
	return json.Marshal(s.CreateAccountResult)
}

// Scan implements the sql.Scanner interface for JSON storage
func (s *SnowflakeAccountDetails) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, &s.CreateAccountResult)
}

// CertificateValidationRecord represents the DNS validation details for a certificate
type CertificateValidationRecord struct {
	DomainName  string `json:"domainName"`
	RecordName  string `json:"recordName"`
	RecordValue string `json:"recordValue"`
	RecordType  string `json:"recordType"`
}

// CertificateRecord represents a registered certificate and its validation details
type CertificateRecord struct {
	CertificateArn    string                      `json:"certificateArn"`
	ValidationDetails CertificateValidationRecord `json:"validationDetails"`
	IsIssued          bool                        `json:"isIssued"`
}

// CertificateResults stores the results of certificate registration
type CertificateResults struct {
	LogSubdomain      *CertificateRecord `json:"logSubdomain,omitempty"`
	PantherSubdomain  *CertificateRecord `json:"pantherSubdomain,omitempty"`
	WildcardSubdomain *CertificateRecord `json:"wildcardSubdomain,omitempty"`
}

// Value implements the driver.Valuer interface for JSON storage
func (c CertificateResults) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface for JSON storage
func (c *CertificateResults) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, &c)
}

// ReadinessCheckResults represents the results of the AWS readiness check
type ReadinessCheckResults struct {
	DeploymentRoleReadinessResults []map[string]interface{} `json:"deployment_role_readiness_results"`
	S3SelectEnabled                bool                     `json:"s3_select_enabled"`
}

func (r *ReadinessCheckResults) HasPassed() bool {
	return r.S3SelectEnabled && len(r.DeploymentRoleReadinessResults) == 0
}

// Value implements the driver.Valuer interface for JSON storage
func (r ReadinessCheckResults) Value() (driver.Value, error) {
	return json.Marshal(r)
}

// Scan implements the sql.Scanner interface for JSON storage
func (r *ReadinessCheckResults) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, &r)
}

type Row struct {
	ConfigHash                         string `validate:"sha256"`
	SnowflakeAdminUsername             string
	SnowflakeAdminPassword             string
	SnowflakeAccountDetails            SnowflakeAccountDetails
	AWSPantherDeploymentRoleDeployed   bool
	AWSReadinessBootstrapToolsDeployed bool
	AWSReadinessCheckSucceeded         bool
	AWSReadinessCheckResults           ReadinessCheckResults
	AWSSnowflakeBootstrapSucceeded     bool
	AWSCertificatesRequested           bool
	AWSCertificatesResults             CertificateResults
}
