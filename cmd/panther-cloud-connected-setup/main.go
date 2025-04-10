package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/aws"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/panther"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/snowflake"
	"github.com/panther-labs/panther-cli/pkg/state"
	"github.com/panther-labs/panther-cli/pkg/util"
	"github.com/pkg/errors"
	_ "github.com/snowflakedb/gosnowflake"

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
	defer func() {
		if err := stateManager.Close(); err != nil {
			log.Fatalf("failed to close state manager: %v\n", err)
		}
	}()

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
			log.Fatalf("AWS readiness check failed - ensure S3 Select is enabled and all deployment role checks pass")
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

		// Update only the Snowflake bootstrap state
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

	// show this run's results
	showLastRun(a.ConfigFile)
}

// writeJSONSupportFile generates a JSON support file using the provided state and config
// It returns the JSON string for possible further use
func writeJSONSupportFile(currentState *state.Row, cfg config.Config) (string, error) {
	// Generate JSON content
	jsonStr, err := currentState.FormatJSON(cfg)
	if err != nil {
		return "", errors.Wrap(err, "failed to format JSON output")
	}

	// Get components for filename
	subdomain := cfg.AWSConfig.DomainCertificateConfiguration.PantherSubdomain
	awsAccountID := cfg.AWSConfig.MustGetAWSAccountID()
	snowflakeLocator := currentState.SnowflakeAccountDetails.AccountName

	// Construct filename
	filename := fmt.Sprintf("%s-%s-%s-supportfile.json", subdomain, awsAccountID, snowflakeLocator)

	// Check if file exists
	if _, err := os.Stat(filename); err == nil {
		// File exists, prompt for overwrite
		log.Printf("File %s already exists. Overwrite? (y/n): ", filename)
		var response string
		n, err := fmt.Scanln(&response)
		// Handle potential Scanln errors
		if err != nil {
			// EOF or unexpected error
			if err.Error() != "unexpected newline" {
				log.Printf("Error reading input: %v", err)
			}
			// Default to not overwriting on error
			log.Println("File not overwritten due to input error.")
			return jsonStr, nil
		}

		// Check if we got any input
		if n == 0 {
			log.Println("No input received. File not overwritten.")
			return jsonStr, nil
		}

		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			log.Println("File not overwritten.")
			return jsonStr, nil
		}
	}

	// Write to file
	err = os.WriteFile(filename, []byte(jsonStr), 0o600)
	if err != nil {
		return jsonStr, errors.Wrapf(err, "failed to write to file %s", filename)
	}

	log.Printf("Support file written to %s\n", filename)
	log.Println("Please share this file with your Panther support team.")
	return jsonStr, nil
}

func showLastRun(configFile string) {
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
	defer func() {
		if err := stateManager.Close(); err != nil {
			log.Fatalf("failed to close state manager: %v\n", err)
		}
	}()

	currentState := stateManager.GetState()

	// If we get here, use the human-readable format
	currentState.PrettyPrint(cfg)

	// now write the JSON support file
	log.Println()
	log.Println()
	jsonStr, err := writeJSONSupportFile(currentState, cfg)
	if err != nil {
		log.Fatalf("Failed to create JSON support file: %v", err)
	}
	util.LogDebugln(jsonStr)
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
			"Some certificates are still pending validation. Please ensure you have created the following DNS records:",
		)
		printDNSValidationInstructions(certs)
	}

	return nil
}

func printDNSValidationInstructions(certs state.CertificateResults) {
	if certs.PantherSubdomain != nil {
		log.Printf(
			"For Panther Subdomain (%s), create a DNS record with the following information:\n",
			certs.PantherSubdomain.ValidationDetails.DomainName,
		)
		log.Printf("  Record Type:  %s\n", certs.PantherSubdomain.ValidationDetails.RecordType)
		log.Printf("  Record Name:  %s\n", certs.PantherSubdomain.ValidationDetails.RecordName)
		log.Printf("  Record Value: %s\n", certs.PantherSubdomain.ValidationDetails.RecordValue)
	}

	if certs.WildcardSubdomain != nil {
		log.Printf(
			"For Wildcard Certificate (%s), create a DNS record with the following information:\n",
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
	defer func() {
		if err := snow.Close(); err != nil {
			log.Fatalf("failed to close Snowflake connection: %v\n", err)
		}
	}()

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
	defer func() {
		if err := snowAcctSetup.Close(); err != nil {
			log.Fatalf("failed to close Snowflake account setup: %v\n", err)
		}
	}()

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
	bootstrap, err := aws.NewLocalSnowflakeCredentialBootstrap(ctx, cfg.AWSConfig)
	if err != nil {
		return "", errors.Wrap(err, "failed to initialize Snowflake credential bootstrap")
	}

	credsARN, err := bootstrap.WriteSecret(ctx, createAcctRes)
	if err != nil {
		return "", errors.Wrap(err, "failed to execute Snowflake credential bootstrap")
	}

	if err := bootstrap.ValidateSecret(ctx); err != nil {
		return "", errors.Wrap(err, "failed to validate Snowflake credentials")
	}

	return credsARN, nil
}
