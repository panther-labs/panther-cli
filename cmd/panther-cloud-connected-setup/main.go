package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
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

	// Setup AWS if not already done. Error handling is done in the function.
	if err := setupAWS(ctx, cfg, stateManager); err != nil {
		log.Fatalf("failed to setup AWS: %v\n", err)
	}

	// Setup the Datalake if not already done. Error handling is done in the function.
	if !a.SkipDatalakeSetup {
		if err := setupDatalake(ctx, cfg, stateManager); err != nil {
			log.Fatalf("failed to setup datalake: %v\n", err)
		}
	} else {
		log.Println("Skipping datalake setup - --skip-datalake-setup specified")
	}

	// We allow users to skip the readiness check, whether it ran successfully or not.
	if !a.SkipAWSReadinessCheck {
		// Run readiness check if not already done
		if err := runReadinessCheck(ctx, cfg, stateManager); err != nil {
			log.Fatalf("failed to run readiness check: %v\n", err)
		}
	} else {
		log.Println("Skipping readiness check - --skip-aws-readiness-check specified")
	}

	if err := setupCertificates(ctx, cfg, stateManager); err != nil {
		log.Fatalf("failed to setup certificates: %v\n", err)
	}

	// Check certificate issuance status
	isIssued, err := checkCertificateStatus(ctx, cfg, stateManager, a.ForceCheckCertificates)
	if err != nil {
		log.Fatalf("failed to check certificate status: %v\n", err)
	}
	if !isIssued {
		// dont print output of the run if the certs are not issued yet
		// customer needs to manually create the DNS records per instructions
		// emitted in checkCertificateStatus. once created the customer can run the
		// command again which will verify the certs are issued,
		// and then the output file will be emitted
		util.LogRedln("Run this program again once the above DNS records have been created")
		return
	}

	// show this run's results
	showLastRun(a.ConfigFile)
}

// writeJSONSupportFile generates a JSON support file using the provided state and config
// It returns the JSON string for possible further use
func writeJSONSupportFile(currentState *state.Row, cfg *config.Config) (string, error) {
	// Generate JSON content
	jsonStr, err := currentState.FormatJSON(cfg)
	if err != nil {
		return "", errors.Wrap(err, "failed to format JSON output")
	}

	// Get components for filename
	subdomain := cfg.AWSConfig.DomainCertificateConfiguration.PantherSubdomain
	awsAccountID := cfg.AWSConfig.MustGetAWSAccountID()
	snowflakeAccountName := currentState.SnowflakeAccountName

	// Construct filename
	filename := fmt.Sprintf("%s-%s-%s-supportfile.json", subdomain, awsAccountID, snowflakeAccountName)

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
