package config

import (
	"errors"
	"log"
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/mcuadros/go-defaults"
	"gopkg.in/yaml.v3"
)

type Config struct {
	SnowflakeOrgConfig        SnowflakeOrgConfig        `yaml:"SnowflakeOrgConfig"        validate:"required"`
	NewAccountConfig          NewAccountConfig          `yaml:"NewAccountConfig"          validate:"required"`
	PantherAccountAdminConfig PantherAccountAdminConfig `yaml:"PantherAccountAdminConfig" validate:"required"`
	AWSConfig                 AWSConfig                 `yaml:"AWSConfig"                 validate:"required"`
}

func (c Config) validate() error {
	return validate.Struct(c)
}

func NewConfigFromPath(path string) (Config, error) {
	var cfg Config

	file, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, err
	}

	// initialize defaults within the config, this is recursive
	defaults.SetDefaults(&cfg)

	if err := cfg.validate(); err != nil {
		var validationErrs validator.ValidationErrors
		if errors.As(err, &validationErrs) {
			for _, err := range validationErrs {
				log.Printf("Config field '%s' failed validation: %s\n", err.Field(), err.ActualTag())
			}
		}
		return cfg, err
	}

	return cfg, nil
}
