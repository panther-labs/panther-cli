package config

import (
	_ "embed"
	"log"
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/k0kubun/pp/v3"
	"github.com/mcuadros/go-defaults"
	"github.com/panther-labs/panther-cli/pkg/util"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/pkg/errors"
)

//go:embed schema.yaml
var schemaBytes []byte

type Config struct {
	AWSConfig            AWSConfig            `yaml:"AWSConfig"            validate:"required"`
	SnowflakeConfig      *SnowflakeConfig     `yaml:"SnowflakeConfig"`
	RedshiftConfig       *RedshiftConfig      `yaml:"RedshiftConfig"`
	PantherAccountConfig PantherAccountConfig `yaml:"PantherAccountConfig" validate:"required"`
}

func (c *Config) IsSnowflake() bool {
	return c.SnowflakeConfig != nil
}

func (c *Config) IsRedshift() bool {
	// Redshift doesn't actually have any config. This isn't ideal
	return !c.IsSnowflake()
}

func (c *Config) GetDatalakeType() string {
	if c.IsSnowflake() {
		return "Snowflake"
	}
	return "Redshift"
}

func NewConfigFromPath(path string) (*Config, error) {
	cfg := &Config{}

	if err := validateConfigSchema(path); err != nil {
		return nil, errors.Wrapf(err, "config schema validation failed")
	}

	configAsStr, err := readFileContent(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read config file '%s'", path)
	}
	configBytes := []byte(configAsStr)

	var rawConfigData interface{}
	if err := yaml.Unmarshal([]byte(configBytes), &rawConfigData); err != nil {
		return nil, errors.Wrapf(err, "failed to parse config file '%s' as YAML", path)
	}

	if err := yaml.Unmarshal(configBytes, &cfg); err != nil {
		// This should ideally not happen if YAML parsing and schema validation passed,
		// but good to have as a safeguard.
		return nil, errors.Wrapf(err, "failed to unmarshal config into struct after schema validation")
	}

	defaults.SetDefaults(cfg) // initialize defaults within the config

	// Setup dependent fields
	cfg.setupAWSConfig()

	if cfg.SnowflakeConfig == nil && cfg.RedshiftConfig == nil {
		return nil, errors.New("no Snowflake or Redshift config found - is the indentation correct in your config?")
	}

	if cfg.IsSnowflake() {
		log.Println("Setting up Snowflake config")
		if err := cfg.setupSnowflakeConfig(); err != nil {
			return nil, errors.Wrapf(err, "failed during Snowflake config setup")
		}
	} else {
		log.Println("Setting up Redshift config")
	}

	if err := cfg.validateConfigStruct(); err != nil {
		return nil, errors.Wrapf(err, "config validation failed")
	}

	if cfg.IsSnowflake() && cfg.IsRedshift() {
		log.Fatalf(
			"This configuration is in an impossible state. Please contact your Panther support team and share your config file.",
		)
	}

	log.Printf("Config loaded and validated successfully from '%s'", path)
	return cfg, nil
}

// validateConfigSchema validates the config file against the schema.
// It's not a receiver because it's used before we've de-seriealized the
// the config into a struct.
func validateConfigSchema(path string) (err error) {
	configAsStr, err := readFileContent(path)
	if err != nil {
		return errors.Wrapf(err, "failed to read config file '%s'", path)
	}
	configBytes := []byte(configAsStr)

	var rawConfigData interface{}
	if err := yaml.Unmarshal([]byte(configBytes), &rawConfigData); err != nil {
		return errors.Wrapf(err, "failed to parse config file '%s' as YAML", path)
	}

	var schemaAsUnmarshalledYAML interface{}
	if err := yaml.Unmarshal(schemaBytes, &schemaAsUnmarshalledYAML); err != nil {
		return errors.Wrapf(err, "failed to unmarshal schema as YAML")
	}

	compiler := jsonschema.NewCompiler()
	const schemaPath = "./schema.yaml" // Adjust path if necessary

	// Register the schema file with the compiler using the reader
	if err := compiler.AddResource(schemaPath, schemaAsUnmarshalledYAML); err != nil {
		return errors.Wrapf(err, "failed to add schema resource '%s'", schemaPath)
	}
	schema, err := compiler.Compile(schemaPath)
	if err != nil {
		return errors.Wrapf(err, "failed to load or compile schema '%s'", schemaPath)
	}

	if err := schema.Validate(rawConfigData); err != nil {
		// Provide more context on validation failure
		var validationErr *jsonschema.ValidationError
		if errors.As(err, &validationErr) {
			log.Printf("Config file '%s' failed schema validation:", path)
			// Pretty print the detailed validation error structure
			if validationErr != nil {
				util.LogDebugf("Raw schema validation error(s):\n%s", pp.Sprintln(validationErr))
				log.Fatalf("Schema validation error: %s", validationErr.Error())
			}
		}
		return errors.Wrapf(err, "config file '%s' failed schema validation", path)
	}
	log.Printf("Config file '%s' passed schema validation.", path)

	return nil
}

func (cfg *Config) setupAWSConfig() {
	cfg.AWSConfig.Region = cfg.PantherAccountConfig.Region
}

func (cfg *Config) setupSnowflakeConfig() (err error) {
	// Set the type of SnowflakeConfig we are using
	if cfg.SnowflakeConfig.NewAccountConfig != nil && cfg.SnowflakeConfig.ExistingAccountConfig != nil {
		// This case should be caught by the schema validation, but adding a check here for safety.
		return errors.New("SnowflakeConfig cannot contain both NewAccountConfig and ExistingAccountConfig")
	}

	if cfg.SnowflakeConfig.NewAccountConfig != nil {
		cfg.SnowflakeConfig.ConfigType = SnowflakeConfigTypeNewAccount

		// Set the region for the NewAccountConfig based on the chosen Panther region.
		cfg.SnowflakeConfig.NewAccountConfig.Region = cfg.PantherAccountConfig.Region

		log.Printf(
			"Using Panther account config admin (email='%s', first name='%s', last name='%s') for new Snowflake account",
			cfg.PantherAccountConfig.AdminEmail,
			cfg.PantherAccountConfig.AdminUserFirstName,
			cfg.PantherAccountConfig.AdminUserLastName,
		)
		cfg.SnowflakeConfig.NewAccountConfig.AdminEmail = cfg.PantherAccountConfig.AdminEmail
		cfg.SnowflakeConfig.NewAccountConfig.AdminUserFirstName = cfg.PantherAccountConfig.AdminUserFirstName
		cfg.SnowflakeConfig.NewAccountConfig.AdminUserLastName = cfg.PantherAccountConfig.AdminUserLastName

		// Read the OrgAdminPrivateKey from file if this is a NewAccountConfig
		if cfg.SnowflakeConfig.NewAccountConfig.OrgConfig.OrgAdminPrivateKeyPath != "" {
			privateKey, err := os.ReadFile(cfg.SnowflakeConfig.NewAccountConfig.OrgConfig.OrgAdminPrivateKeyPath)
			if err != nil {
				// Wrap error for better context
				return errors.Wrapf(err,
					"failed to read OrgAdminPrivateKey from path '%s'",
					cfg.SnowflakeConfig.NewAccountConfig.OrgConfig.OrgAdminPrivateKeyPath,
				)
			}
			cfg.SnowflakeConfig.NewAccountConfig.OrgConfig.OrgAdminPrivateKey = string(privateKey)
		}
	} else if cfg.SnowflakeConfig.ExistingAccountConfig != nil {
		cfg.SnowflakeConfig.ConfigType = SnowflakeConfigTypeExistingAccount
		// Potentially read ExistingAccountConfig private key here if needed later? (Added for completeness, no logic needed now)
		// if cfg.SnowflakeConfig.ExistingAccountConfig.PantherAccountAdminRSAKeyPath != "" { ... }
	} else {
		// This case should also be caught by schema validation (oneOf).
		log.Printf("Error: No SnowflakeConfig or invalid SnowflakeConfig provided. This should have been caught by schema validation.\n%s", pp.Sprint(cfg.SnowflakeConfig))
		return errors.New("invalid SnowflakeConfig: must contain either NewAccountConfig or ExistingAccountConfig")
	}

	return nil
}

func (cfg *Config) validateConfigStruct() (err error) {
	if err := validate.Struct(cfg); err != nil {
		var validationErrs validator.ValidationErrors
		if errors.As(err, &validationErrs) {
			log.Println("Struct validation failed on the following fields:") // More informative log
			for _, fe := range validationErrs {
				// Provide more detail from the validation error
				log.Printf("  Field: '%s', Tag: '%s', Value: '%v'\n", fe.Namespace(), fe.Tag(), fe.Value())
			}
		} else {
			// Log unexpected error types from validator
			log.Printf("An unexpected error occurred during struct validation: %v\n", err)
		}
		// Return the original error for upstream handling
		return errors.Wrapf(err, "struct field validation failed")
	}
	return nil
}

func readFileContent(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read file '%s'", filePath)
	}
	return string(content), nil
}
