package state

import (
	"database/sql/driver"
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/snowflake"
	"github.com/panther-labs/panther-cli/pkg/rsapem"
)

// SnowflakeAccountDetails wraps snowflake.CreateAccountResult for JSON serialization
type SnowflakeAccountDetails struct {
	snowflake.CreateAccountResult
}

type serializedSnowflakeAccountDetails struct {
	snowflake.CreateAccountResult
	SerializedKey string
}

// Value implements the driver.Valuer interface for JSON storage
// and handles the RSA key with a dedicated encoder
func (s SnowflakeAccountDetails) Value() (driver.Value, error) {
	serializedKey, err := rsapem.EncodeRSAPEMPrivateKey(s.AdminRSAKey)
	if err != nil {
		return nil, errors.Wrap(err, "serializing AdminRSAKey in row Valuer")
	}
	return json.Marshal(serializedSnowflakeAccountDetails{
		CreateAccountResult: s.CreateAccountResult,
		SerializedKey:       serializedKey,
	})
}

// Scan implements the sql.Scanner interface for JSON storage
// and handles the RSA key with a dedicated decoder
func (s *SnowflakeAccountDetails) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	x := serializedSnowflakeAccountDetails{}
	err := json.Unmarshal(b, &x)
	if err != nil {
		return errors.Wrap(err, "unmarshalling SnowflakeAccountDetails")
	}
	s.CreateAccountResult = x.CreateAccountResult
	s.AdminRSAKey, err = rsapem.ParseRSAPEMPrivateKey(x.SerializedKey)
	return err
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
	return len(r.DeploymentRoleReadinessResults) == 0
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
	SnowflakeAdminRSAKey               string
	SnowflakeAccountDetails            SnowflakeAccountDetails
	AWSPantherDeploymentRoleDeployed   bool
	AWSReadinessBootstrapToolsDeployed bool
	AWSReadinessCheckSucceeded         bool
	AWSReadinessCheckResults           ReadinessCheckResults
	AWSSnowflakeBootstrapSucceeded     bool
	AWSSnowflakeSecretARN              string
	AWSCertificatesRequested           bool
	AWSCertificatesResults             CertificateResults
}
