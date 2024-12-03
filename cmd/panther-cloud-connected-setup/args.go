package main

type args struct {
	ConfigFile              string `arg:"required,-c,--config-file" help:"Configuration file"`
	Verbose                 bool   `arg:"-v,--verbose"              help:"Enable verbose logging"`
	VerboseSnowflakeLogging bool   `arg:"--snowflake-logging"       help:"Enable verbose Snowflake logging (very noisy)"`
}
