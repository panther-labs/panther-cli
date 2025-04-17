package config

import (
	"errors"
	"log"
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/k0kubun/pp/v3"
	"github.com/mcuadros/go-defaults"
	"gopkg.in/yaml.v3"
)

type Config struct {
	AWSConfig            AWSConfig            `yaml:"AWSConfig"            validate:"required"`
	SnowflakeConfig      SnowflakeConfig      `yaml:"SnowflakeConfig"      validate:"required"`
	PantherAccountConfig PantherAccountConfig `yaml:"PantherAccountConfig" validate:"required"`
}

func (c Config) validate() error {
	return validate.Struct(c)
}

func NewConfigFromPath(path string) (*Config, error) {
	cfg := &Config{}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Fatalf("failed to close config file: %v\n", err)
		}
	}()

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	// initialize defaults within the config, this is recursive
	defaults.SetDefaults(cfg)

	// We need the region, but the customer doesn't need to provide it twice. It
	// should always match up between the two configs.
	if err := setupAWSConfig(cfg); err != nil {
		return nil, err
	}

	if err := setupSnowflakeConfig(cfg); err != nil {
		return nil, err
	}

	log.Printf("Config:\n%s", pp.Sprint(cfg))

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func setupAWSConfig(cfg *Config) (err error) {
	cfg.AWSConfig.Region = cfg.PantherAccountConfig.Region
	return nil
}

func setupSnowflakeConfig(cfg *Config) (err error) {
	// Set the type of SnowflakeConfig we are using
	if !cfg.SnowflakeConfig.NewAccountConfig.IsEmpty() {
		cfg.SnowflakeConfig.ConfigType = SnowflakeConfigTypeNewAccount

		// Set the region for the NewAccountConfig based on the chosen Panther region.
		cfg.SnowflakeConfig.NewAccountConfig.Region = cfg.PantherAccountConfig.Region
	} else if !cfg.SnowflakeConfig.ExistingAccountConfig.IsEmpty() {
		cfg.SnowflakeConfig.ConfigType = SnowflakeConfigTypeExistingAccount
	} else {
		log.Fatalf("No SnowflakeConfig or invalid SnowflakeConfig provided:\n%s", pp.Sprint(cfg.SnowflakeConfig))
	}

	// Read the OrgAdminPrivateKey from file if this is a NewAccountConfig
	if cfg.SnowflakeConfig.NewAccountConfig.OrgConfig.OrgAdminPrivateKeyPath != "" {
		privateKey, err := os.ReadFile(cfg.SnowflakeConfig.NewAccountConfig.OrgConfig.OrgAdminPrivateKeyPath)
		if err != nil {
			return err
		}
		cfg.SnowflakeConfig.NewAccountConfig.OrgConfig.OrgAdminPrivateKey = string(privateKey)
	}

	return nil
}

func validateConfig(cfg *Config) (err error) {
	if err := cfg.validate(); err != nil {
		var validationErrs validator.ValidationErrors
		if errors.As(err, &validationErrs) {
			for _, err := range validationErrs {
				log.Printf("Config field '%s' failed validation: %s\n", err.Field(), err.ActualTag())
			}
		}
		return err
	}

	return nil
}
