package config

type DatabricksConfig struct {
	Enabled bool `yaml:"Enabled" required:"true"`
}
