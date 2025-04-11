package config

//nolint:lll
type PantherAccountConfig struct {
	DesiredPantherAccountName string   `yaml:"DesiredPantherAccountName" validate:"required"`
	Edition                   string   `yaml:"PantherEdition"            validate:"required,oneof=ENTERPRISE ESSENTIALS"` // if PantherEdition==Enterprise, SnowflakeEdition must be Enterprise
	Region                    string   `yaml:"PantherRegion"             validate:"required,validPantherRegion"`          // this informs which Snowflake region we use, currently AWS-only
	IpAddressAllowList        []string `yaml:"IpAddressAllowList"        validate:"omitempty,dive,ip"`                    // Optional list of allowed IP addresses
}
