package config

type RedshiftConfig struct {
	Enabled bool `yaml:"Enabled" required:"true"`
}
