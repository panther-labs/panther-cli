package config

//nolint:lll
type PantherAccountConfig struct {
	AdminEmail         string   `yaml:"AdminEmail"         validate:"required,email"`
	AdminUserFirstName string   `yaml:"AdminUserFirstName" validate:"required"`
	AdminUserLastName  string   `yaml:"AdminUserLastName"  validate:"required"`
	DesiredAccountName string   `yaml:"DesiredAccountName" validate:"required"`
	Edition            string   `yaml:"Edition"            validate:"required,oneof=ENTERPRISE ESSENTIALS"` // if PantherEdition==Enterprise, SnowflakeEdition must be Enterprise
	Region             string   `yaml:"Region"             validate:"required,validPantherRegion"`          // this informs which Snowflake region we use, currently AWS-only
	IpAddressAllowList []string `yaml:"IpAddressAllowList" validate:"omitempty,dive,cidr"`                  // Optional list of allowed IP addresses
}
