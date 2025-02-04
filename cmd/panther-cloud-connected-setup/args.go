package main

type args struct {
	ConfigFile              string `arg:"-c,--config-file"    help:"Configuration file"`
	Verbose                 bool   `arg:"-v,--verbose"        help:"Enable verbose logging"`
	VerboseSnowflakeLogging bool   `arg:"--snowflake-logging" help:"Enable verbose Snowflake logging (very noisy)"`
	ShowLastRun             bool   `arg:"--show-last-run"     help:"Show the results of the last run"`
	JSONOutput              bool   `arg:"--json"              help:"Output in JSON format (only applies to --show-last-run)"`
	Clean                   bool   `arg:"--clean"             help:"Remove the state database file"`
}
