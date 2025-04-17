package config

//nolint:lll
type PantherAccountConfig struct {
	DesiredAccountName string   `yaml:"DesiredAccountName" validate:"required"`
	Edition            string   `yaml:"Edition"            validate:"required,oneof=ENTERPRISE ESSENTIALS"` // if PantherEdition==Enterprise, SnowflakeEdition must be Enterprise
	Region             string   `yaml:"Region"             validate:"required,validPantherRegion"`          // this informs which Snowflake region we use, currently AWS-only
	IpAddressAllowList []string `yaml:"IpAddressAllowList" validate:"omitempty,dive,ip"`                    // Optional list of allowed IP addresses
}
