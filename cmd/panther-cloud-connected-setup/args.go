package main

import (
	"log"
	"os"

	"github.com/alexflint/go-arg"
	"github.com/panther-labs/panther-cli/pkg/state"
)

type args struct {
	ConfigFile              string `arg:"-c,--config-file"           help:"Configuration file"`
	Verbose                 bool   `arg:"-v,--verbose"               help:"Enable verbose logging"`
	VerboseSnowflakeLogging bool   `arg:"--snowflake-logging"        help:"Enable verbose Snowflake logging (very noisy)"`
	ShowLastRun             bool   `arg:"--show-last-run"            help:"Show the results of the last run"`
	Clean                   bool   `arg:"--clean"                    help:"Remove the state database file"`
	SkipAWSReadinessCheck   bool   `arg:"--skip-aws-readiness-check" help:"Skip the AWS readiness check"`
}

// validateArgs checks that the arguments passed to the program
// are coherent. If they are, it'll return the args struct.
func validateArgs() (a args) {
	p := arg.MustParse(&a)

	if a.Clean && a.ConfigFile != "" {
		p.Fail("cannot specify both --clean and --config-file")
	}

	if !a.Clean && a.ConfigFile == "" && !a.ShowLastRun {
		p.Fail("must specify a command")
	}

	// Handle clean command
	if a.Clean {
		if err := state.CleanState(); err != nil {
			log.Fatalf("failed to clean state: %v\n", err)
		}
		log.Println("Successfully cleaned state")
		os.Exit(0)
	}

	// Handle show-last-run command
	if a.ShowLastRun {
		showLastRun(a.ConfigFile)
		os.Exit(0)
	}

	if a.Verbose {
		if err := os.Setenv("DEBUG", "true"); err != nil {
			log.Fatalf("failed to set DEBUG environment variable: %v\n", err)
		}
	}

	if a.VerboseSnowflakeLogging {
		if err := os.Setenv("SNOWFLAKE_DEBUG", "true"); err != nil {
			log.Fatalf("failed to set SNOWFLAKE_DEBUG environment variable: %v\n", err)
		}
	}

	return a
}
