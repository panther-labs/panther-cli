package main

import (
	"context"
	"log"

	"github.com/cenkalti/backoff/v4"
	pp "github.com/k0kubun/pp/v3"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/aws"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/panther"
	"github.com/panther-labs/panther-cli/pkg/state"
	"github.com/panther-labs/panther-cli/pkg/util"
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

		// Attempt to auto-register validation domains
		pantherAutoReg, err := certHelper.RegisterValidationDomains(pantherResult.ValidationDetails)
		if err != nil {
			util.LogWarnf("Failed to auto-register validation domains for panther certificate: %v", err)
			util.LogWarnln("You will need to manually create the DNS validation records.")
		}

		if err := stateManager.UpdateCertificateState("panther", pantherResult, pantherAutoReg, false); err != nil {
			return errors.Wrap(err, "failed to update panther certificate state")
		}

		if cfg.AWSConfig.Region != "us-east-1" {
			// Register wildcard certificate
			wildcardResult, err := certHelper.RegisterWildcardSubdomainCertificate()
			if err != nil {
				return errors.Wrap(err, "failed to register wildcard certificate")
			}
			log.Printf("Registered wildcard certificate:\n%s\n", pp.Sprintln(wildcardResult))

			wildcardAutoReg, err := certHelper.RegisterValidationDomains(wildcardResult.ValidationDetails)
			if err != nil {
				util.LogWarnf("Failed to auto-register validation domains for wildcard certificate: %v", err)
				util.LogWarnln("You will need to manually create the DNS validation records.")
			}

			if err := stateManager.UpdateCertificateState("wildcard", wildcardResult, wildcardAutoReg, false); err != nil {
				return errors.Wrap(err, "failed to update wildcard certificate state")
			}
		} else {
			log.Printf("Using panther certificate as wildcard certificate in us-east-1 region\n")
			if err := stateManager.UpdateCertificateState("wildcard", pantherResult, pantherAutoReg, false); err != nil {
				return errors.Wrap(err, "failed to update wildcard certificate state")
			}
		}

	}

	return nil
}

// pollCertificateUntilIssued polls a certificate until it's issued using exponential backoff
// Returns true if certificate was issued, false if timeout reached
func pollCertificateUntilIssued(
	ctx context.Context,
	certHelper *aws.CertificateRegistrationHelper,
	certificateArn string,
	isWildcard bool,
	certType string,
) (bool, error) {
	log.Printf("Polling %s certificate for issuance with exponential backoff...", certType)

	var issued bool
	operation := func() error {
		if ctx.Err() != nil {
			util.LogDebugf("Certificate polling stopped due to context cancellation: %v", ctx.Err())
			return backoff.Permanent(ctx.Err())
		}

		var err error
		issued, err = certHelper.IsCertificateIssued(certificateArn, isWildcard)
		if err != nil {
			return backoff.Permanent(errors.Wrapf(err, "failed to check %s certificate status during polling", certType))
		}

		if issued {
			util.LogDebugf("%s certificate has been issued", certType)
			return nil
		}

		return errors.Errorf("%s certificate is not yet issued", certType)
	}

	err := backoff.Retry(operation, util.GetDefaultExponentialBackoffRetrier())
	if err != nil {
		log.Printf("Polling timeout reached for %s certificate", certType)
		return false, nil
	}
	return true, nil
}

func checkCertificateStatus(ctx context.Context, cfg *config.Config, stateManager *state.Manager, forceCheck bool) error {
	certHelper, err := aws.NewCertificateRegistrationHelper(ctx, cfg)
	if err != nil {
		return errors.Wrap(err, "failed to initialize certificate registration helper")
	}

	state := stateManager.GetState()
	certs := state.AWSCertificatesResults

	if certs.PantherSubdomain != nil && (!certs.PantherSubdomain.IsIssued || forceCheck) {
		if forceCheck && certs.PantherSubdomain.IsIssued {
			log.Println("Force checking panther subdomain certificate status (already marked as issued)")
		}

		issued, err := pollCertificateUntilIssued(
			ctx, certHelper, certs.PantherSubdomain.CertificateArn, false, "panther",
		)
		if err != nil {
			return errors.Wrap(err, "failed to check panther subdomain certificate status")
		}

		if issued {
			if err := stateManager.UpdateCertificateState(
				"panther",
				aws.CertificateRegistrationResult{
					CertificateArn: certs.PantherSubdomain.CertificateArn,
					ValidationDetails: aws.CertificateValidationDetails{
						DomainNames: certs.PantherSubdomain.ValidationDetails.DomainNames,
						RecordName:  certs.PantherSubdomain.ValidationDetails.RecordName,
						RecordValue: certs.PantherSubdomain.ValidationDetails.RecordValue,
						RecordType:  certs.PantherSubdomain.ValidationDetails.RecordType,
					},
				},
				aws.AutoRegistrationResult{
					Attempted: certs.PantherSubdomain.AutoRegistrationAttempted,
					Succeeded: certs.PantherSubdomain.AutoRegistrationSucceeded,
				},
				true,
			); err != nil {
				return errors.Wrap(err, "failed to update panther certificate state")
			}
			log.Println("Panther subdomain certificate has been issued")
		} else {
			log.Println("Panther subdomain certificate is still pending after polling timeout")
		}
	}

	// Check wildcard certificate
	if certs.WildcardSubdomain != nil && (!certs.WildcardSubdomain.IsIssued || forceCheck) {
		if forceCheck && certs.WildcardSubdomain.IsIssued {
			log.Println("Force checking wildcard certificate status (already marked as issued)")
		}

		issued, err := pollCertificateUntilIssued(
			ctx, certHelper, certs.WildcardSubdomain.CertificateArn, true, "wildcard",
		)
		if err != nil {
			return errors.Wrap(err, "failed to check wildcard certificate status")
		}

		if issued {
			if err := stateManager.UpdateCertificateState(
				"wildcard",
				aws.CertificateRegistrationResult{
					CertificateArn: certs.WildcardSubdomain.CertificateArn,
					ValidationDetails: aws.CertificateValidationDetails{
						DomainNames: certs.WildcardSubdomain.ValidationDetails.DomainNames,
						RecordName:  certs.WildcardSubdomain.ValidationDetails.RecordName,
						RecordValue: certs.WildcardSubdomain.ValidationDetails.RecordValue,
						RecordType:  certs.WildcardSubdomain.ValidationDetails.RecordType,
					},
				},
				aws.AutoRegistrationResult{
					Attempted: certs.WildcardSubdomain.AutoRegistrationAttempted,
					Succeeded: certs.WildcardSubdomain.AutoRegistrationSucceeded,
				},
				true,
			); err != nil {
				return errors.Wrap(err, "failed to update wildcard certificate state")
			}
			log.Println("Wildcard certificate has been issued")
		} else {
			log.Println("Wildcard certificate is still pending after polling timeout")
		}
	}

	// Refresh state after potential updates
	state = stateManager.GetState()
	certs = state.AWSCertificatesResults

	// If any certificates are not issued, print the DNS validation instructions
	if (certs.PantherSubdomain != nil && !certs.PantherSubdomain.IsIssued) ||
		(certs.WildcardSubdomain != nil && !certs.WildcardSubdomain.IsIssued) {
		log.Println(
			"Some certificates are still pending validation. Please ensure you have created the following DNS records:",
		)
		printDNSValidationInstructions(certs)
	}

	return nil
}

func printDNSValidationInstructions(certs state.CertificateResults) {
	if certs.PantherSubdomain != nil {
		domainName := "unknown"
		if len(certs.PantherSubdomain.ValidationDetails.DomainNames) > 0 {
			domainName = certs.PantherSubdomain.ValidationDetails.DomainNames[0]
		}
		log.Printf(
			"For Panther Subdomain (%s), create a DNS record with the following information:\n",
			domainName,
		)
		log.Printf("  Record Type:  %s\n", certs.PantherSubdomain.ValidationDetails.RecordType)
		log.Printf("  Record Name:  %s\n", certs.PantherSubdomain.ValidationDetails.RecordName)
		log.Printf("  Record Value: %s\n", certs.PantherSubdomain.ValidationDetails.RecordValue)
	}

	if certs.WildcardSubdomain != nil {
		domainName := "unknown"
		if len(certs.WildcardSubdomain.ValidationDetails.DomainNames) > 0 {
			domainName = certs.WildcardSubdomain.ValidationDetails.DomainNames[0]
		}
		log.Printf(
			"For Wildcard Certificate (%s), create a DNS record with the following information:\n",
			domainName,
		)
		log.Printf("  Record Type:  %s\n", certs.WildcardSubdomain.ValidationDetails.RecordType)
		log.Printf("  Record Name:  %s\n", certs.WildcardSubdomain.ValidationDetails.RecordName)
		log.Printf("  Record Value: %s\n", certs.WildcardSubdomain.ValidationDetails.RecordValue)
	}
}

func setupAWS(ctx context.Context, cfg *config.Config, stateManager *state.Manager) error {
	currentState := stateManager.GetState()

	if !currentState.IsAWSSetup() {
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
		if !currentState.AWSPantherDeploymentRoleUpdaterDeployed {
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
