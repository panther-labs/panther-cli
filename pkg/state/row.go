package state

import (
	"database/sql/driver"
	"encoding/json"
	"log"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/snowflake"
)

// CertificateValidationRecord represents the DNS validation details for a certificate
type CertificateValidationRecord struct {
	DomainName  string `json:"domain_name"`
	RecordName  string `json:"record_name"`
	RecordValue string `json:"record_value"`
	RecordType  string `json:"record_type"`
}

// CertificateRecord represents a registered certificate and its validation details
type CertificateRecord struct {
	CertificateArn    string                      `json:"certificate_arn"`
	ValidationDetails CertificateValidationRecord `json:"validation_details"`
	IsIssued          bool                        `json:"is_issued"`
}

// CertificateResults stores the results of certificate registration
type CertificateResults struct {
	PantherSubdomain  *CertificateRecord `json:"panther_subdomain,omitempty"`
	WildcardSubdomain *CertificateRecord `json:"wildcard_subdomain,omitempty"`
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
	ConfigHash             string `validate:"sha256"`
	SnowflakeAccountName   string
	SnowflakeAccountURL    string
	SnowflakeEdition       string
	SnowflakeRegion        string
	SnowflakeAccountSetup  bool
	SnowflakeAdminUsername string

	AWSPantherDeploymentRoleDeployed        bool
	AWSPantherDeploymentRoleUpdaterDeployed bool
	AWSReadinessBootstrapToolsDeployed      bool
	AWSReadinessCheckSucceeded              bool
	AWSReadinessCheckResults                ReadinessCheckResults
	AWSSnowflakeBootstrapSucceeded          bool
	AWSSnowflakeSecretARN                   string
	AWSCertificatesRequested                bool
	AWSCertificatesResults                  CertificateResults
}

// OutputDetails contains the structured information for formatted output
type OutputDetails struct {
	DesiredPantherAccountName string   `json:"desired_panther_account_name"`
	PantherSubdomain          string   `json:"panther_subdomain"`
	PantherEdition            string   `json:"panther_edition"`
	PantherRegion             string   `json:"panther_region"`
	IpAddressAllowList        []string `json:"ip_address_allow_list,omitempty"`

	AdminUserFirstName string `json:"admin_user_first_name"`
	AdminUserLastName  string `json:"admin_user_last_name"`
	AdminEmail         string `json:"admin_email"`

	SnowflakeSecretARN     string `json:"snowflake_secret_arn,omitempty"`
	SnowflakeAccountName   string `json:"snowflake_account_name,omitempty"`
	SnowflakeAccountURL    string `json:"snowflake_account_url,omitempty"`
	SnowflakeEdition       string `json:"snowflake_edition,omitempty"`
	SnowflakeRegion        string `json:"snowflake_region,omitempty"`
	SnowflakeAdminUsername string `json:"snowflake_admin_username,omitempty"`
	SnowflakeAdminSetup    bool   `json:"snowflake_admin_setup,omitempty"`
	AWSAccountID           string `json:"aws_account_id"`

	PantherCertificate  *CertificateRecord     `json:"panther_certificate,omitempty"`
	WildcardCertificate *CertificateRecord     `json:"wildcard_certificate,omitempty"`
	DeploymentStatus    map[string]interface{} `json:"deployment_status"`
}

// PrettyPrint outputs a human-readable format of the state to the standard logger
// It requires a config.Config to access some information.
func (r *Row) PrettyPrint(cfg *config.Config) {
	// Get structured output data
	output := r.createStructuredOutput(cfg)

	// Create a title caser for English
	titleCaser := cases.Title(language.English)

	// Print Snowflake Account Details section
	log.Printf("Snowflake Account Details:\n")
	log.Printf("  Account Name: %s\n", r.SnowflakeAccountName)
	log.Printf("  URL: %s\n", r.SnowflakeAccountURL)
	log.Printf("  Edition: %s\n", r.SnowflakeEdition)

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
		// Using the modern title caser instead of deprecated strings.Title
		formattedKey = titleCaser.String(formattedKey)
		log.Printf("  %s: %v\n", formattedKey, value)
	}

	// Print Snowflake Secret ARN if available and not "unknown"
	if output.SnowflakeSecretARN != "" && output.SnowflakeSecretARN != "unknown" {
		log.Printf("Snowflake Secret ARN: %s\n", output.SnowflakeSecretARN)
	}

	// Print Certificate Status section if certificates exist
	if output.PantherCertificate != nil || output.WildcardCertificate != nil {
		log.Printf("Certificate Status:\n")

		if output.PantherCertificate != nil {
			log.Printf("  Panther Subdomain Certificate:\n")
			log.Printf("    ARN: %s\n", output.PantherCertificate.CertificateArn)
			// Get certificate issuance status directly from the struct
			log.Printf("    Issued: %v\n", output.PantherCertificate.IsIssued)
			// Optionally print validation details if needed
			log.Printf("    Validation Details: %+v\n", output.PantherCertificate.ValidationDetails)
		}

		if output.WildcardCertificate != nil {
			log.Printf("  Wildcard Certificate:\n")
			log.Printf("    ARN: %s\n", output.WildcardCertificate.CertificateArn)
			// Get certificate issuance status directly from the struct
			log.Printf("    Issued: %v\n", output.WildcardCertificate.IsIssued)
			// Optionally print validation details if needed
			log.Printf("    Validation Details: %+v\n", output.WildcardCertificate.ValidationDetails)
		}
	}
}

func (r *Row) PopulateSnowflakeAccountDetails(accountDetails *snowflake.ResolvedSnowflakeAcccount) {
	fullyQualifiedAccountName, err := accountDetails.GetFullyQualifiedAccountName()
	if err != nil {
		log.Printf("failed to get fully qualified account name, error='%s'", err)
	}
	r.SnowflakeAccountName = fullyQualifiedAccountName
	r.SnowflakeAccountURL = accountDetails.URL
	r.SnowflakeEdition = accountDetails.Edition
	r.SnowflakeRegion = accountDetails.Region
	r.SnowflakeAdminUsername = accountDetails.AdminUsername
}

func (r *Row) RenderNonSensitiveSnowflakeAccountDetails() *snowflake.ResolvedSnowflakeAcccount {
	return &snowflake.ResolvedSnowflakeAcccount{
		AccountName:   r.SnowflakeAccountName,
		URL:           r.SnowflakeAccountURL,
		Edition:       r.SnowflakeEdition,
		Region:        r.SnowflakeRegion,
		AdminUsername: r.SnowflakeAdminUsername,
	}
}

// FormatJSON returns a formatted JSON string representation of the row.
// It requires a config.Config to generate the output.
func (r *Row) FormatJSON(cfg *config.Config) (string, error) {
	// Create structured output
	output := r.createStructuredOutput(cfg)

	// Marshal to JSON with indentation
	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal output to JSON")
	}

	return string(jsonData), nil
}

func (r *Row) IsAWSSetup() bool {
	return r.AWSPantherDeploymentRoleDeployed &&
		r.AWSReadinessBootstrapToolsDeployed &&
		r.AWSPantherDeploymentRoleUpdaterDeployed
}

// createStructuredOutput creates a structured output object with relevant information
func (r *Row) createStructuredOutput(cfg *config.Config) OutputDetails {
	// Initialize the output structure
	output := OutputDetails{
		AWSAccountID:              cfg.AWSConfig.MustGetAWSAccountID(),
		PantherSubdomain:          cfg.AWSConfig.DomainCertificateConfiguration.PantherSubdomain,
		DesiredPantherAccountName: cfg.PantherAccountConfig.DesiredAccountName,
		AdminUserFirstName:        cfg.PantherAccountConfig.AdminUserFirstName,
		AdminUserLastName:         cfg.PantherAccountConfig.AdminUserLastName,
		AdminEmail:                cfg.PantherAccountConfig.AdminEmail,
		IpAddressAllowList:        cfg.PantherAccountConfig.IpAddressAllowList,
		SnowflakeAccountName:      r.SnowflakeAccountName,
		SnowflakeAccountURL:       r.SnowflakeAccountURL,
		SnowflakeEdition:          r.SnowflakeEdition,
		SnowflakeRegion:           r.SnowflakeRegion,
		SnowflakeAdminUsername:    r.SnowflakeAdminUsername,
		PantherEdition:            cfg.PantherAccountConfig.Edition,
		PantherRegion:             cfg.PantherAccountConfig.Region,
		PantherCertificate:        r.AWSCertificatesResults.PantherSubdomain,
		WildcardCertificate:       r.AWSCertificatesResults.WildcardSubdomain,
		DeploymentStatus:          make(map[string]interface{}),
	}

	// Add deployment status information
	output.DeploymentStatus["aws_deployment_role_deployed"] = r.AWSPantherDeploymentRoleDeployed
	output.DeploymentStatus["aws_deployment_role_updater_deployed"] = r.AWSPantherDeploymentRoleUpdaterDeployed
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
