package state

import (
	"database/sql/driver"
	"encoding/json"
	"log"
	"strings"

	"github.com/pkg/errors"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
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

// OutputDetails contains the structured information for formatted output
type OutputDetails struct {
	AWSAccountID           string                 `json:"aws_account_id"`
	PantherSubdomain       string                 `json:"panther_subdomain"`
	SnowflakeSecretARN     string                 `json:"snowflake_secret_arn"`
	SnowflakeRegion        string                 `json:"snowflake_region"`
	SnowflakeEdition       string                 `json:"snowflake_edition"`
	PantherCertificateARN  string                 `json:"panther_certificate_arn,omitempty"`
	WildcardCertificateARN string                 `json:"wildcard_certificate_arn,omitempty"`
	DeploymentStatus       map[string]interface{} `json:"deployment_status"`
}

// PrettyPrint outputs a human-readable format of the state to the standard logger
// It requires a config.Config to access some information.
func (r *Row) PrettyPrint(cfg config.Config) {
	// Get structured output data
	output := r.createStructuredOutput(cfg)

	// Print Snowflake Account Details section
	log.Printf("Snowflake Account Details:\n")
	log.Printf("  Account Name: %s\n", r.SnowflakeAccountDetails.AccountName)
	log.Printf("  URL: %s\n", r.SnowflakeAccountDetails.URL)
	log.Printf("  Admin Username: %s\n", r.SnowflakeAdminUsername)
	log.Printf("  Region: %s\n", output.SnowflakeRegion)
	log.Printf("  Edition: %s\n", output.SnowflakeEdition)

	// Print AWS Account ID if available
	if output.AWSAccountID != "" {
		log.Printf("AWS Account ID: %s\n", output.AWSAccountID)
	}

	// Print Panther Subdomain if available
	if output.PantherSubdomain != "" {
		log.Printf("Panther Subdomain: %s\n", output.PantherSubdomain)
	}

	// Print AWS Deployment Status section using map iteration
	log.Printf("AWS Deployment Status:\n")
	for key, value := range output.DeploymentStatus {
		// Format keys for better readability by replacing underscores with spaces and capitalizing
		formattedKey := strings.ReplaceAll(key, "_", " ")
		formattedKey = strings.Title(formattedKey)
		log.Printf("  %s: %v\n", formattedKey, value)
	}

	// Print Snowflake Secret ARN if available and not "unknown"
	if output.SnowflakeSecretARN != "" && output.SnowflakeSecretARN != "unknown" {
		log.Printf("Snowflake Secret ARN: %s\n", output.SnowflakeSecretARN)
	}

	// Print Certificate Status section if certificates exist
	if output.PantherCertificateARN != "" || output.WildcardCertificateARN != "" {
		log.Printf("Certificate Status:\n")

		if output.PantherCertificateARN != "" {
			log.Printf("  Panther Subdomain Certificate:\n")
			log.Printf("    ARN: %s\n", output.PantherCertificateARN)
			// Get certificate issuance status which isn't in the OutputDetails
			isIssued := false
			if r.AWSCertificatesResults.PantherSubdomain != nil {
				isIssued = r.AWSCertificatesResults.PantherSubdomain.IsIssued
			}
			log.Printf("    Issued: %v\n", isIssued)
		}

		if output.WildcardCertificateARN != "" {
			log.Printf("  Wildcard Certificate:\n")
			log.Printf("    ARN: %s\n", output.WildcardCertificateARN)
			// Get certificate issuance status which isn't in the OutputDetails
			isIssued := false
			if r.AWSCertificatesResults.WildcardSubdomain != nil {
				isIssued = r.AWSCertificatesResults.WildcardSubdomain.IsIssued
			}
			log.Printf("    Issued: %v\n", isIssued)
		}
	}
}

// FormatJSON returns a formatted JSON string representation of the row.
// It requires a config.Config to generate the output.
func (r *Row) FormatJSON(cfg config.Config) (string, error) {
	// Create structured output
	output := r.createStructuredOutput(cfg)

	// Marshal to JSON with indentation
	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal output to JSON")
	}

	return string(jsonData), nil
}

// createStructuredOutput creates a structured output object with relevant information
func (r *Row) createStructuredOutput(cfg config.Config) OutputDetails {
	// Initialize the output structure
	output := OutputDetails{
		AWSAccountID:     cfg.AWSConfig.MustGetAWSAccountID(),
		PantherSubdomain: cfg.AWSConfig.DomainCertificateConfiguration.PantherSubdomain,
		SnowflakeRegion:  cfg.NewAccountConfig.PantherRegion,
		SnowflakeEdition: cfg.NewAccountConfig.SnowflakeEdition,
		DeploymentStatus: make(map[string]interface{}),
	}

	// Add certificate ARNs if available
	if r.AWSCertificatesResults.PantherSubdomain != nil {
		output.PantherCertificateARN = r.AWSCertificatesResults.PantherSubdomain.CertificateArn
	}
	if r.AWSCertificatesResults.WildcardSubdomain != nil {
		output.WildcardCertificateARN = r.AWSCertificatesResults.WildcardSubdomain.CertificateArn
	}

	// Add deployment status information
	output.DeploymentStatus["aws_deployment_role_deployed"] = r.AWSPantherDeploymentRoleDeployed
	output.DeploymentStatus["aws_bootstrap_tools_deployed"] = r.AWSReadinessBootstrapToolsDeployed
	output.DeploymentStatus["aws_readiness_check_succeeded"] = r.AWSReadinessCheckSucceeded
	output.DeploymentStatus["aws_snowflake_bootstrap_succeeded"] = r.AWSSnowflakeBootstrapSucceeded

	// Use the ARN from state if available
	if r.AWSSnowflakeBootstrapSucceeded && r.AWSSnowflakeSecretARN != "" {
		output.SnowflakeSecretARN = r.AWSSnowflakeSecretARN
	} else {
		output.SnowflakeSecretARN = "unknown"
	}

	return output
}
