package main

import (
	"context"
	"log"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/aws"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/snowflake"
	"github.com/panther-labs/panther-cli/pkg/state"
	"github.com/pkg/errors"
)

func setupDatalake(ctx context.Context, cfg *config.Config, stateManager *state.Manager) error {
	currentState := stateManager.GetState()

	if cfg.IsSnowflake() {
		log.Println("Snowflake deployment specified.")

		if currentState.SnowflakeAccountName == "" {
			// Setup Snowflake if not already done
			snowflakeSetup := snowflake.NewSnowflakeSetup(ctx, cfg)
			resolvedSnowflakeAccount, err := snowflakeSetup.CreateOrResolveAccount()
			if err != nil {
				return errors.Wrap(err, "failed to create or resolve Snowflake account")
			}

			if err := stateManager.UpdateSnowflakeState(resolvedSnowflakeAccount, false); err != nil {
				return errors.Wrap(err, "failed to update Snowflake state")
			}
			log.Printf("Successfully resolved Snowflake account: %s\n", resolvedSnowflakeAccount.URL)

			if err := snowflakeSetup.SetupAccount(resolvedSnowflakeAccount); err != nil {
				return errors.Wrap(err, "failed to setup Snowflake account")
			}

			if err := stateManager.UpdateSnowflakeState(resolvedSnowflakeAccount, true); err != nil {
				return errors.Wrap(err, "failed to update Snowflake state")
			}
		} else {
			log.Printf("Using existing Snowflake account details: %s\n", currentState.SnowflakeAccountURL)
		}

		resolvedSnowflakeAccount := currentState.RenderNonSensitiveSnowflakeAccountDetails()

		privateKey, err := cfg.SnowflakeConfig.GetPantherAccountAdminRSAKey()
		if err != nil {
			return errors.Wrap(err, "failed to get PANTHERACCOUNTADMIN RSA key")
		}
		resolvedSnowflakeAccount.AdminRSAKey = privateKey

		// Run Snowflake credential bootstrap if not already done
		if !currentState.AWSSnowflakeBootstrapSucceeded {
			credsARN, err := runSnowflakeCredentialBootstrap(ctx, cfg, resolvedSnowflakeAccount)
			if err != nil {
				return errors.Wrap(err, "failed to run Snowflake credential bootstrap")
			}

			// Update only the Snowflake bootstrap state
			if err := stateManager.UpdateAWSSnowflakeBootstrapState(true, credsARN); err != nil {
				return errors.Wrap(err, "failed to update AWS Snowflake bootstrap state")
			}
			log.Printf("Snowflake credential bootstrap completed successfully. Credentials ARN: %s\n", credsARN)
		} else {
			log.Println("Skipping Snowflake credential bootstrap - already completed")
		}
	} else if cfg.IsRedshift() {
		log.Println("Redshift deployment specified.")
	}

	log.Printf("Datalake(%s) setup completed successfully.", cfg.GetDatalakeType())

	return nil
}

func runSnowflakeCredentialBootstrap(
	ctx context.Context,
	cfg *config.Config,
	resolvedSnowflakeAcct *snowflake.ResolvedSnowflakeAcccount,
) (string, error) {
	bootstrap, err := aws.NewLocalSnowflakeCredentialBootstrap(ctx, cfg.AWSConfig)
	if err != nil {
		return "", errors.Wrap(err, "failed to initialize Snowflake credential bootstrap")
	}

	credsARN, err := bootstrap.WriteSecret(ctx, resolvedSnowflakeAcct)
	if err != nil {
		return "", errors.Wrap(err, "failed to execute Snowflake credential bootstrap")
	}

	if err := bootstrap.ValidateSecret(ctx); err != nil {
		return "", errors.Wrap(err, "failed to validate Snowflake credentials")
	}

	return credsARN, nil
}
