package main

import (
	"context"
	"log"

	pp "github.com/k0kubun/pp/v3"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/aws"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/panther"
	"github.com/panther-labs/panther-cli/pkg/state"
	"github.com/pkg/errors"
)

func setupCertificates(ctx context.Context, cfg *config.Config, stateManager *state.Manager) error {
	currentState := stateManager.GetState()
	if !currentState.AWSCertificatesRequested {
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
	}

	return nil
}

func checkCertificateStatus(ctx context.Context, cfg *config.Config, stateManager *state.Manager) error {
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

func setupAWS(ctx context.Context, cfg *config.Config, stateManager *state.Manager) error {
	currentState := stateManager.GetState()

	needsSetup := !currentState.AWSPantherDeploymentRoleDeployed ||
		!currentState.AWSReadinessBootstrapToolsDeployed ||
		!currentState.AWSDeploymentRoleUpdaterDeployed

	if needsSetup {
		awsSetup, err := aws.NewCloudFormation(ctx, cfg.AWSConfig)
		if err != nil {
			return errors.Wrap(err, "failed to initialize AWS CloudFormation")
		}

		if !currentState.AWSPantherDeploymentRoleDeployed {
			log.Println("Deploying PantherDeploymentRole...")
			if err := awsSetup.ApplyDeploymentRole(); err != nil {
				return errors.Wrapf(
					err,
					"failed to create deployment role stack (%s)",
					cfg.AWSConfig.CloudFormationConfig.DeploymentRoleName,
				)
			}

			if err := stateManager.UpdateAWSDeploymentState(true); err != nil {
				return errors.Wrap(err, "failed to update AWS deployment state")
			}

			log.Println("Successfully deployed PantherDeploymentRole")
		} else {
			log.Println("Using existing PantherDeploymentRole")
		}

		if !currentState.AWSReadinessBootstrapToolsDeployed {
			log.Println("Deploying pre-deployment tools...")
			if err := awsSetup.ApplyPreDeploymentTools(); err != nil {
				return errors.Wrapf(
					err,
					"failed to apply pre-deployment tools stack (%s)",
					cfg.AWSConfig.CloudFormationConfig.PreDeploymentToolsStackName,
				)
			}

			if err := stateManager.UpdateAWSBootstrapState(true); err != nil {
				return errors.Wrap(err, "failed to update AWS bootstrap state")
			}

			log.Println("Successfully deployed pre-deployment tools")
		} else {
			log.Println("Using existing pre-deployment tools")
		}

		// Deploy Deployment Role Updater if needed
		if !currentState.AWSDeploymentRoleUpdaterDeployed {
			log.Println("Deploying deployment role updater...")
			if err := awsSetup.ApplyDeploymentRoleUpdater(); err != nil {
				return errors.Wrapf(
					err,
					"failed to apply deployment role updater stack (%s)",
					cfg.AWSConfig.CloudFormationConfig.DeploymentRoleUpdaterStackName,
				)
			}

			if err := stateManager.UpdateAWSDeploymentRoleUpdaterState(true); err != nil {
				return errors.Wrap(err, "failed to update AWS deployment role updater state")
			}

			log.Println("Successfully deployed deployment role updater")
		} else {
			log.Println("Using existing deployment role updater")
		}
	} else {
		log.Println("Using existing AWS setup")
	}

	return nil
}

func runReadinessCheck(ctx context.Context, cfg *config.Config, stateManager *state.Manager) error {
	currentState := stateManager.GetState()

	if !currentState.AWSReadinessCheckSucceeded {
		readinessCheck, err := panther.NewReadinessCheck(ctx, cfg.AWSConfig)
		if err != nil {
			return errors.Wrap(err, "failed to initialize readiness check")
		}

		result, err := readinessCheck.Exec()
		if err != nil {
			return errors.Wrap(err, "failed to execute readiness check")
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

		results := state.ReadinessCheckResults{
			DeploymentRoleReadinessResults: deploymentRoleResults,
			S3SelectEnabled:                s3Enabled,
		}

		if err := stateManager.UpdateAWSReadinessState(results); err != nil {
			return errors.Wrap(err, "failed to update AWS readiness state")
		}

		if !results.HasPassed() {
			return errors.Errorf(
				"AWS readiness check failed - ensure all deployment role checks pass:\n%s",
				pp.Sprintln(results),
			)
		}
	} else {
		log.Println("Skipping readiness check - already completed")
	}

	return nil
}
