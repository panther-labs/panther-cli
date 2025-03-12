package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/aws"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/panther"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/snowflake"
	"github.com/panther-labs/panther-cli/pkg/state"
	"github.com/panther-labs/panther-cli/pkg/util"
	"github.com/pkg/errors"
	_ "github.com/snowflakedb/gosnowflake"
	"gopkg.in/yaml.v3"

	pp "github.com/k0kubun/pp/v3"
)

func init() {
	pp.Default.SetColoringEnabled(true)
}

func main() {
	a := validateArgs()

	cfg, err := config.NewConfigFromPath(a.ConfigFile)
	if err != nil {
		log.Fatalf("failed to load account configuration: %v\n", err)
	}

	util.LogDebugln(pp.Sprintln(cfg))

	// Initialize state manager
	stateManager, err := state.NewManager(cfg)
	if err != nil {
		log.Fatalf("failed to initialize state manager: %v\n", err)
	}
	defer stateManager.Close()

	ctx := context.Background()
	currentState := stateManager.GetState()
	util.LogDebugln(pp.Sprintln(currentState))

	// Setup Snowflake if not already done
	var createAcctRes snowflake.CreateAccountResult
	if currentState.SnowflakeAccountDetails.AccountName == "" {
		createAcctRes, err = setupSnowflakeAccount(ctx, cfg)
		if err != nil {
			log.Fatalf("failed to setup Snowflake: %v\n", err)
		}
		if err := stateManager.UpdateSnowflakeState(
			cfg.NewAccountConfig.AdminUsername,
			cfg.NewAccountConfig.AdminPassword,
			createAcctRes,
		); err != nil {
			log.Fatalf("failed to update Snowflake state: %v\n", err)
		}
	} else {
		createAcctRes = currentState.SnowflakeAccountDetails.CreateAccountResult
		log.Println("Using existing Snowflake setup")
	}
	// idempotent, fine to run again if this is restarting from state
	err = snowflakeAdminUserSetup(ctx, createAcctRes, cfg)
	if err != nil {
		log.Fatalf("failed to create Snowflake admin user: %v\n", err)
	}

	// Setup AWS if not already done
	if !currentState.AWSPantherDeploymentRoleDeployed || !currentState.AWSReadinessBootstrapToolsDeployed {
		if err := setupAWS(ctx, cfg); err != nil {
			log.Fatalf("failed to setup AWS: %v\n", err)
		}
		if err := stateManager.UpdateAWSDeploymentState(true); err != nil {
			log.Fatalf("failed to update AWS deployment state: %v\n", err)
		}
		if err := stateManager.UpdateAWSBootstrapState(true); err != nil {
			log.Fatalf("failed to update AWS bootstrap state: %v\n", err)
		}
	} else {
		log.Println("Using existing AWS setup")
	}

	// Run readiness check if not already done
	if !currentState.AWSReadinessCheckSucceeded {
		results, err := runReadinessCheck(ctx, cfg)
		if err != nil {
			log.Fatalf("failed to run readiness check: %v\n", err)
		}
		if err := stateManager.UpdateAWSReadinessState(results); err != nil {
			log.Fatalf("failed to update AWS readiness state: %v\n", err)
		}
		if !results.HasPassed() {
			log.Fatalf("AWS readiness check failed")
		}
	} else {
		log.Println("Skipping readiness check - already completed")
	}

	// Run Snowflake credential bootstrap if not already done
	if !currentState.AWSSnowflakeBootstrapSucceeded {
		credsARN, err := runSnowflakeCredentialBootstrap(ctx, cfg, createAcctRes)
		if err != nil {
			log.Fatalf("failed to run Snowflake credential bootstrap: %v\n", err)
		}

		// Update the AWS Snowflake bootstrap state with success flag and secret ARN
		if err := stateManager.UpdateAWSSnowflakeBootstrapState(true, credsARN); err != nil {
			log.Fatalf("failed to update AWS Snowflake bootstrap state: %v\n", err)
		}
		log.Printf("Snowflake credential bootstrap completed successfully. Credentials ARN: %s\n", credsARN)
	} else {
		log.Println("Skipping Snowflake credential bootstrap - already completed")
	}

	// Setup certificates if not already done or check issuance status
	if !currentState.AWSCertificatesRequested {
		if err := setupCertificates(ctx, cfg, stateManager); err != nil {
			log.Fatalf("failed to setup certificates: %v\n", err)
		}
	} else {
		if err := checkCertificateStatus(ctx, cfg, stateManager); err != nil {
			log.Fatalf("failed to check certificate status: %v\n", err)
		}
	}
}

// OutputDetails contains the structured information for formatted output
type OutputDetails struct {
	AWSAccountID           string                 `json:"aws_account_id"                     yaml:"aws_account_id"`
	PantherSubdomain       string                 `json:"panther_subdomain"                  yaml:"panther_subdomain"`
	SnowflakeSecretARN     string                 `json:"snowflake_secret_arn"               yaml:"snowflake_secret_arn"`
	SnowflakeRegion        string                 `json:"snowflake_region"                   yaml:"snowflake_region"`
	SnowflakeEdition       string                 `json:"snowflake_edition"                  yaml:"snowflake_edition"`
	PantherCertificateARN  string                 `json:"panther_certificate_arn,omitempty"  yaml:"panther_certificate_arn,omitempty"`
	WildcardCertificateARN string                 `json:"wildcard_certificate_arn,omitempty" yaml:"wildcard_certificate_arn,omitempty"`
	DeploymentStatus       map[string]interface{} `json:"deployment_status"                  yaml:"deployment_status"`
}

func showLastRun(configFile string, jsonOutput bool, yamlOutput bool) {
	if !state.HasState() {
		log.Fatalf("No state found. Run the setup process first to create state.")
	}

	cfg, err := config.NewConfigFromPath(configFile)
	if err != nil {
		log.Fatalf("failed to load account configuration: %v\n", err)
	}

	stateManager, err := state.NewManager(cfg)
	if err != nil {
		log.Fatalf("failed to initialize state manager: %v\n", err)
	}
	defer stateManager.Close()

	currentState := stateManager.GetState()

	// If JSON or YAML output is requested, use structured output
	if jsonOutput || yamlOutput {
		// Create structured output
		ctx := context.Background()
		output := createStructuredOutput(ctx, cfg, currentState)

		if jsonOutput {
			// Output JSON
			jsonData, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				log.Fatalf("Failed to marshal output to JSON: %v", err)
			}
			fmt.Println(string(jsonData))
		} else {
			// Output YAML
			yamlData, err := yaml.Marshal(output)
			if err != nil {
				log.Fatalf("Failed to marshal output to YAML: %v", err)
			}
			fmt.Println(string(yamlData))
		}
		os.Exit(0)
	}

	// If we get here, use the original human-readable format
	log.Printf("Snowflake Account Details:\n")
	log.Printf("  Account Name: %s\n", currentState.SnowflakeAccountDetails.AccountName)
	log.Printf("  URL: %s\n", currentState.SnowflakeAccountDetails.URL)
	log.Printf("  Admin Username: %s\n", currentState.SnowflakeAdminUsername)
	log.Printf("  Region: %s\n", currentState.SnowflakeAccountDetails.Region)
	log.Printf("  Edition: %s\n", currentState.SnowflakeAccountDetails.Edition)

	log.Printf("\nAWS Deployment Status:\n")
	log.Printf("  Deployment Role Deployed: %v\n", currentState.AWSPantherDeploymentRoleDeployed)
	log.Printf("  Bootstrap Tools Deployed: %v\n", currentState.AWSReadinessBootstrapToolsDeployed)
	log.Printf("  Readiness Check Succeeded: %v\n", currentState.AWSReadinessCheckSucceeded)
	log.Printf("  Snowflake Bootstrap Succeeded: %v\n", currentState.AWSSnowflakeBootstrapSucceeded)

	if currentState.AWSCertificatesRequested {
		log.Printf("\nCertificate Status:\n")
		if currentState.AWSCertificatesResults.PantherSubdomain != nil {
			log.Printf("  Panther Subdomain Certificate:\n")
			log.Printf("    ARN: %s\n", currentState.AWSCertificatesResults.PantherSubdomain.CertificateArn)
			log.Printf("    Issued: %v\n", currentState.AWSCertificatesResults.PantherSubdomain.IsIssued)
		}
		if currentState.AWSCertificatesResults.WildcardSubdomain != nil {
			log.Printf("  Wildcard Certificate:\n")
			log.Printf("    ARN: %s\n", currentState.AWSCertificatesResults.WildcardSubdomain.CertificateArn)
			log.Printf("    Issued: %v\n", currentState.AWSCertificatesResults.WildcardSubdomain.IsIssued)
		}
	}
}

// createStructuredOutput creates a structured output object with all required information
func createStructuredOutput(ctx context.Context, cfg config.Config, currentState *state.Row) OutputDetails {
	// Initialize the output structure
	output := OutputDetails{
		AWSAccountID:     cfg.AWSConfig.CloudFormationConfig.IdentityAccountId,
		PantherSubdomain: cfg.AWSConfig.DomainCertificateConfiguration.PantherSubdomain,
		SnowflakeRegion:  cfg.NewAccountConfig.PantherRegion,
		SnowflakeEdition: cfg.NewAccountConfig.SnowflakeEdition,
		DeploymentStatus: make(map[string]interface{}),
	}

	// Add certificate ARNs if available
	if currentState.AWSCertificatesResults.PantherSubdomain != nil {
		output.PantherCertificateARN = currentState.AWSCertificatesResults.PantherSubdomain.CertificateArn
	}
	if currentState.AWSCertificatesResults.WildcardSubdomain != nil {
		output.WildcardCertificateARN = currentState.AWSCertificatesResults.WildcardSubdomain.CertificateArn
	}

	// Add deployment status information
	output.DeploymentStatus["aws_deployment_role_deployed"] = currentState.AWSPantherDeploymentRoleDeployed
	output.DeploymentStatus["aws_bootstrap_tools_deployed"] = currentState.AWSReadinessBootstrapToolsDeployed
	output.DeploymentStatus["aws_readiness_check_succeeded"] = currentState.AWSReadinessCheckSucceeded
	output.DeploymentStatus["aws_snowflake_bootstrap_succeeded"] = currentState.AWSSnowflakeBootstrapSucceeded

	// Get the Snowflake secret ARN from state if available, otherwise try to retrieve it
	if currentState.AWSSnowflakeBootstrapSucceeded && currentState.AWSSnowflakeSecretARN != "" {
		output.SnowflakeSecretARN = currentState.AWSSnowflakeSecretARN
	} else {
		// Try to get the ARN from AWS Secrets Manager
		secretARN, err := getSnowflakeSecretARN(ctx, cfg)
		if err != nil {
			log.Printf("Warning: Failed to retrieve Snowflake secret ARN: %v", err)
			output.SnowflakeSecretARN = "unknown"
		} else {
			output.SnowflakeSecretARN = secretARN
		}
	}

	return output
}

// getSnowflakeSecretARN retrieves the ARN of the Snowflake secret
func getSnowflakeSecretARN(ctx context.Context, cfg config.Config) (string, error) {
	// Create AWS config
	awsCfg, err := util.GetAWSConfig(
		ctx,
		cfg.AWSConfig.AccessKeyID,
		cfg.AWSConfig.SecretAccessKey,
		cfg.AWSConfig.SessionToken,
	)
	if err != nil {
		return "", errors.Wrap(err, "failed to create AWS config")
	}

	// Create Secrets Manager client
	sm := secretsmanager.NewFromConfig(awsCfg, func(o *secretsmanager.Options) {
		o.Region = cfg.AWSConfig.Region
	})

	// Get secret information
	secretName := "panther-managed-accountadmin-secret"
	input := &secretsmanager.DescribeSecretInput{
		SecretId: awssdk.String(secretName),
	}

	secretInfo, err := sm.DescribeSecret(ctx, input)
	if err != nil {
		return "", errors.Wrap(err, "failed to describe Snowflake secret")
	}

	return *secretInfo.ARN, nil
}

func setupCertificates(ctx context.Context, cfg config.Config, stateManager *state.Manager) error {
	certHelper, err := aws.NewCertificateRegistrationHelper(ctx, cfg)
	if err != nil {
		return errors.Wrap(err, "failed to initialize certificate registration helper")
	}

	// Register panther subdomain certificate
	pantherResult, err := certHelper.RegisterPantherSubdomainCertificate()
	if err != nil {
		return errors.Wrap(err, "failed to register panther subdomain certificate")
	}
	log.Printf("Registered panther subdomain certificate:\n%s\n", pp.Sprintln(pantherResult))
	if err := stateManager.UpdateCertificateState("panther", pantherResult, false); err != nil {
		return errors.Wrap(err, "failed to update panther certificate state")
	}

	// Register wildcard certificate
	wildcardResult, err := certHelper.RegisterWildcardSubdomainCertificate()
	if err != nil {
		return errors.Wrap(err, "failed to register wildcard certificate")
	}
	log.Printf("Registered wildcard certificate:\n%s\n", pp.Sprintln(wildcardResult))
	if err := stateManager.UpdateCertificateState("wildcard", wildcardResult, false); err != nil {
		return errors.Wrap(err, "failed to update wildcard certificate state")
	}

	// Print DNS validation instructions
	printDNSValidationInstructions(stateManager.GetState().AWSCertificatesResults)

	return nil
}

func checkCertificateStatus(ctx context.Context, cfg config.Config, stateManager *state.Manager) error {
	certHelper, err := aws.NewCertificateRegistrationHelper(ctx, cfg)
	if err != nil {
		return errors.Wrap(err, "failed to initialize certificate registration helper")
	}

	state := stateManager.GetState()
	certs := state.AWSCertificatesResults

	// Check panther subdomain certificate
	if certs.PantherSubdomain != nil && !certs.PantherSubdomain.IsIssued {
		issued, err := certHelper.IsCertificateIssued(certs.PantherSubdomain.CertificateArn, false)
		if err != nil {
			return errors.Wrap(err, "failed to check panther subdomain certificate status")
		}
		if issued {
			if err := stateManager.UpdateCertificateState(
				"panther",
				aws.CertificateRegistrationResult{
					CertificateArn: certs.PantherSubdomain.CertificateArn,
					ValidationDetails: aws.CertificateValidationDetails{
						DomainName:  certs.PantherSubdomain.ValidationDetails.DomainName,
						RecordName:  certs.PantherSubdomain.ValidationDetails.RecordName,
						RecordValue: certs.PantherSubdomain.ValidationDetails.RecordValue,
						RecordType:  certs.PantherSubdomain.ValidationDetails.RecordType,
					},
				},
				true,
			); err != nil {
				return errors.Wrap(err, "failed to update panther certificate state")
			}
			log.Println("Panther subdomain certificate has been issued")
		}
	}

	// Check wildcard certificate
	if certs.WildcardSubdomain != nil && !certs.WildcardSubdomain.IsIssued {
		issued, err := certHelper.IsCertificateIssued(certs.WildcardSubdomain.CertificateArn, true)
		if err != nil {
			return errors.Wrap(err, "failed to check wildcard certificate status")
		}
		if issued {
			if err := stateManager.UpdateCertificateState(
				"wildcard",
				aws.CertificateRegistrationResult{
					CertificateArn: certs.WildcardSubdomain.CertificateArn,
					ValidationDetails: aws.CertificateValidationDetails{
						DomainName:  certs.WildcardSubdomain.ValidationDetails.DomainName,
						RecordName:  certs.WildcardSubdomain.ValidationDetails.RecordName,
						RecordValue: certs.WildcardSubdomain.ValidationDetails.RecordValue,
						RecordType:  certs.WildcardSubdomain.ValidationDetails.RecordType,
					},
				},
				true,
			); err != nil {
				return errors.Wrap(err, "failed to update wildcard certificate state")
			}
			log.Println("Wildcard certificate has been issued")
		}
	}

	// If any certificates are not issued, print the DNS validation instructions
	if (!certs.PantherSubdomain.IsIssued && certs.PantherSubdomain != nil) ||
		(!certs.WildcardSubdomain.IsIssued && certs.WildcardSubdomain != nil) {
		log.Println(
			"\nSome certificates are still pending validation. Please ensure you have created the following DNS records:",
		)
		printDNSValidationInstructions(certs)
	}

	return nil
}

func printDNSValidationInstructions(certs state.CertificateResults) {
	if certs.PantherSubdomain != nil {
		log.Printf(
			"\nFor Panther Subdomain (%s), create a DNS record with the following information:\n",
			certs.PantherSubdomain.ValidationDetails.DomainName,
		)
		log.Printf("  Record Type:  %s\n", certs.PantherSubdomain.ValidationDetails.RecordType)
		log.Printf("  Record Name:  %s\n", certs.PantherSubdomain.ValidationDetails.RecordName)
		log.Printf("  Record Value: %s\n", certs.PantherSubdomain.ValidationDetails.RecordValue)
	}

	if certs.WildcardSubdomain != nil {
		log.Printf(
			"\nFor Wildcard Certificate (%s), create a DNS record with the following information:\n",
			certs.WildcardSubdomain.ValidationDetails.DomainName,
		)
		log.Printf("  Record Type:  %s\n", certs.WildcardSubdomain.ValidationDetails.RecordType)
		log.Printf("  Record Name:  %s\n", certs.WildcardSubdomain.ValidationDetails.RecordName)
		log.Printf("  Record Value: %s\n", certs.WildcardSubdomain.ValidationDetails.RecordValue)
	}
}

// Uses orgadmin (provided with RSA key) to create a new account whose first admin user is
// PANTHERACCOUNTADMIN with a newly generated RSA key.
func setupSnowflakeAccount(ctx context.Context, cfg config.Config) (snowflake.CreateAccountResult, error) {
	snow := snowflake.AccountCreate{}

	if err := snow.Connect(ctx, cfg.SnowflakeOrgConfig); err != nil {
		return snowflake.CreateAccountResult{}, errors.Wrap(err, "failed to connect to Snowflake")
	}
	defer snow.Close()

	createAcctRes, err := snow.CreateNewSnowflakeAccount(cfg.NewAccountConfig)
	if err != nil {
		return snowflake.CreateAccountResult{}, errors.Wrap(err, "failed to create new Snowflake account")
	}

	return createAcctRes, nil
}

// Creates a type=person admin user for the customer based on the provided config.
func snowflakeAdminUserSetup(
	ctx context.Context,
	createAcctRes snowflake.CreateAccountResult,
	cfg config.Config,
) error {
	snowAcctSetup := snowflake.AccountSetup{}
	if err := snowAcctSetup.Connect(ctx, createAcctRes); err != nil {
		return errors.Wrap(err, "failed to connect to new Snowflake account")
	}
	defer snowAcctSetup.Close()

	if err := snowAcctSetup.SetupCustomerAccountAdminUser(cfg.NewAccountConfig); err != nil {
		return errors.Wrap(err, "failed to setup Panther account admin user")
	}
	return nil
}

func setupAWS(ctx context.Context, cfg config.Config) error {
	awsSetup, err := aws.NewCloudFormation(ctx, cfg.AWSConfig)
	if err != nil {
		return errors.Wrap(err, "failed to initialize AWS CloudFormation")
	}

	if err := awsSetup.ApplyDeploymentRole(); err != nil {
		return errors.Wrapf(
			err,
			"failed to create deployment role stack (%s)",
			cfg.AWSConfig.CloudFormationConfig.DeploymentRoleName,
		)
	}

	if err := awsSetup.ApplyPreDeploymentTools(); err != nil {
		return errors.Wrapf(
			err,
			"failed to apply pre-deployment tools stack (%s)",
			cfg.AWSConfig.CloudFormationConfig.PreDeploymentToolsStackName,
		)
	}

	return nil
}

func runReadinessCheck(ctx context.Context, cfg config.Config) (state.ReadinessCheckResults, error) {
	readinessCheck, err := panther.NewReadinessCheck(ctx, cfg.AWSConfig)
	if err != nil {
		return state.ReadinessCheckResults{}, errors.Wrap(err, "failed to initialize readiness check")
	}

	result, err := readinessCheck.Exec()
	if err != nil {
		return state.ReadinessCheckResults{}, errors.Wrap(err, "failed to execute readiness check")
	}

	log.Println(pp.Sprintln(result))

	// Convert the map result to ReadinessCheckResults
	deploymentResults, _ := result["deployment_role_readiness_results"].([]interface{})
	s3Enabled, _ := result["s3_select_enabled"].(bool)

	// Convert []interface{} to []map[string]interface{}
	var deploymentRoleResults []map[string]interface{}
	for _, item := range deploymentResults {
		if m, ok := item.(map[string]interface{}); ok {
			deploymentRoleResults = append(deploymentRoleResults, m)
		}
	}

	return state.ReadinessCheckResults{
		DeploymentRoleReadinessResults: deploymentRoleResults,
		S3SelectEnabled:                s3Enabled,
	}, nil
}

func runSnowflakeCredentialBootstrap(
	ctx context.Context,
	cfg config.Config,
	createAcctRes snowflake.CreateAccountResult,
) (string, error) {
	bootstrap, err := aws.NewPantherSnowflakeCredentialBootstrap(ctx, cfg)
	if err != nil {
		return "", errors.Wrap(err, "failed to initialize Snowflake credential bootstrap")
	}

	credsARN, err := bootstrap.Exec(createAcctRes)
	if err != nil {
		return "", errors.Wrap(err, "failed to execute Snowflake credential bootstrap")
	}

	return credsARN, nil
}
