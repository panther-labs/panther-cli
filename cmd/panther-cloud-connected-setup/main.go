package main

import (
	"context"
	"log"
	"os"

	"github.com/alexflint/go-arg"
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
	var a args
	arg.MustParse(&a)

	ctx := context.Background()

	cfg, err := config.NewConfigFromPath(a.ConfigFile)
	if err != nil {
		log.Fatalf("failed to load account configuration: %v\n", err)
	}

	util.LogDebugln(pp.Sprintln(cfg))

	if a.Verbose {
		os.Setenv("DEBUG", "true")
	}

	if a.VerboseSnowflakeLogging {
		os.Setenv("SNOWFLAKE_DEBUG", "true")
	}

	// Initialize state manager
	stateManager, err := state.NewManager(cfg)
	if err != nil {
		log.Fatalf("failed to initialize state manager: %v\n", err)
	}
	defer stateManager.Close()

	currentState := stateManager.GetState()

	// Setup Snowflake if not already done
	var createAcctRes snowflake.CreateAccountResult
	if currentState.SnowflakeAccountDetails.AccountLocator == "" {
		createAcctRes, err = setupSnowflake(ctx, cfg)
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
	} else {
		log.Println("Skipping readiness check - already completed")
	}

	// Run Snowflake credential bootstrap if not already done
	if !currentState.AWSSnowflakeBootstrapSucceeded {
		if err := runSnowflakeCredentialBootstrap(ctx, cfg, createAcctRes); err != nil {
			log.Fatalf("failed to run Snowflake credential bootstrap: %v\n", err)
		}
		if err := stateManager.UpdateAWSSnowflakeBootstrapState(true); err != nil {
			log.Fatalf("failed to update AWS Snowflake bootstrap state: %v\n", err)
		}
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

func setupCertificates(ctx context.Context, cfg config.Config, stateManager *state.Manager) error {
	certHelper, err := aws.NewCertificateRegistrationHelper(ctx, cfg)
	if err != nil {
		return errors.Wrap(err, "failed to initialize certificate registration helper")
	}

	// Register log subdomain certificate
	logResult, err := certHelper.RegisterLogSubdomainCertificate()
	if err != nil {
		return errors.Wrap(err, "failed to register log subdomain certificate")
	}
	log.Printf("Registered log subdomain certificate:\n%s\n", pp.Sprintln(logResult))
	if err := stateManager.UpdateCertificateState("log", logResult, false); err != nil {
		return errors.Wrap(err, "failed to update log certificate state")
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

	// Check log subdomain certificate
	if certs.LogSubdomain != nil && !certs.LogSubdomain.IsIssued {
		issued, err := certHelper.IsCertificateIssued(certs.LogSubdomain.CertificateArn)
		if err != nil {
			return errors.Wrap(err, "failed to check log subdomain certificate status")
		}
		if issued {
			if err := stateManager.UpdateCertificateState(
				"log",
				aws.CertificateRegistrationResult{
					CertificateArn: certs.LogSubdomain.CertificateArn,
					ValidationDetails: aws.CertificateValidationDetails{
						DomainName:  certs.LogSubdomain.ValidationDetails.DomainName,
						RecordName:  certs.LogSubdomain.ValidationDetails.RecordName,
						RecordValue: certs.LogSubdomain.ValidationDetails.RecordValue,
						RecordType:  certs.LogSubdomain.ValidationDetails.RecordType,
					},
				},
				true,
			); err != nil {
				return errors.Wrap(err, "failed to update log certificate state")
			}
			log.Println("Log subdomain certificate has been issued")
		}
	}

	// Check panther subdomain certificate
	if certs.PantherSubdomain != nil && !certs.PantherSubdomain.IsIssued {
		issued, err := certHelper.IsCertificateIssued(certs.PantherSubdomain.CertificateArn)
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
		issued, err := certHelper.IsCertificateIssued(certs.WildcardSubdomain.CertificateArn)
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

	// If any certificates are not issued, print the DNS validation instructions again
	if !certs.LogSubdomain.IsIssued || !certs.PantherSubdomain.IsIssued || !certs.WildcardSubdomain.IsIssued {
		log.Println(
			"\nSome certificates are still pending validation. Please ensure you have created the following DNS records:",
		)
		printDNSValidationInstructions(certs)
	}

	return nil
}

func printDNSValidationInstructions(certs state.CertificateResults) {
	if certs.LogSubdomain != nil {
		log.Printf(
			"\nFor Log Subdomain (%s), create a DNS record with the following information:\n",
			certs.LogSubdomain.ValidationDetails.DomainName,
		)
		log.Printf("  Record Type:  %s\n", certs.LogSubdomain.ValidationDetails.RecordType)
		log.Printf("  Record Name:  %s\n", certs.LogSubdomain.ValidationDetails.RecordName)
		log.Printf("  Record Value: %s\n", certs.LogSubdomain.ValidationDetails.RecordValue)
	}

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

func setupSnowflake(ctx context.Context, cfg config.Config) (snowflake.CreateAccountResult, error) {
	snow := snowflake.AccountCreate{}

	if err := snow.Connect(ctx, cfg.SnowflakeOrgConfig); err != nil {
		return snowflake.CreateAccountResult{}, errors.Wrap(err, "failed to connect to Snowflake")
	}
	defer snow.Close()

	createAcctRes, err := snow.CreateNewSnowflakeAccount(cfg.NewAccountConfig)
	if err != nil {
		return snowflake.CreateAccountResult{}, errors.Wrap(err, "failed to create new Snowflake account")
	}

	log.Println(pp.Sprintln(createAcctRes))

	snowAcctSetup := snowflake.AccountSetup{}

	if err := snowAcctSetup.Connect(ctx, createAcctRes, cfg.NewAccountConfig.AdminUsername, cfg.NewAccountConfig.AdminPassword); err != nil {
		return snowflake.CreateAccountResult{}, errors.Wrap(err, "failed to connect to new Snowflake account")
	}
	defer snowAcctSetup.Close()

	if err := snowAcctSetup.SetupPantherAccountAdminUser(cfg.PantherAccountAdminConfig); err != nil {
		return snowflake.CreateAccountResult{}, errors.Wrap(err, "failed to setup Panther account admin user")
	}

	return createAcctRes, nil
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
) error {
	bootstrap, err := aws.NewPantherSnowflakeCredentialBootstrap(ctx, cfg)
	if err != nil {
		return errors.Wrap(err, "failed to initialize Snowflake credential bootstrap")
	}

	if err := bootstrap.Exec(createAcctRes); err != nil {
		return errors.Wrap(err, "failed to execute Snowflake credential bootstrap")
	}

	return nil
}
