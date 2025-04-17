package config

import (
	"regexp"
	"slices"

	"github.com/go-playground/validator/v10"
	"github.com/panther-labs/panther-cli/pkg/util"
)

var validate *validator.Validate = validator.New(validator.WithRequiredStructEnabled())

func init() {
	util.Must(
		validate.RegisterValidation("validAcctName", validateSnowflakeAccountName),
		"couldn't register validAcctName validation",
	)
	util.Must(validate.RegisterValidation("validAdminName", validateAdminName),
		"couldn't register validAdminName validation",
	)
	util.Must(validate.RegisterValidation("validPantherRegion", validatePantherRegion),
		"couldn't register validPantherRegion validation",
	)

	util.Must(validate.RegisterValidation("validSnowflakeRegion", validateSnowflakeRegion),
		"couldn't register validSnowflakeRegion validation",
	)

	validate.RegisterStructValidation(validateEditionsMatch, Config{})
}

func validateSnowflakeAccountName(fl validator.FieldLevel) bool {
	accountNameRegex := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	return accountNameRegex.MatchString(fl.Field().String())
}

func validateAdminName(fl validator.FieldLevel) bool {
	adminNameRegex := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	return adminNameRegex.MatchString(fl.Field().String())
}

func validateEditionsMatch(sl validator.StructLevel) {
	cfg := sl.Current().Interface().(Config)

	var specifiedEdition string
	if cfg.SnowflakeConfig.NewAccountConfig != nil {
		specifiedEdition = cfg.SnowflakeConfig.NewAccountConfig.Edition
	} else if cfg.SnowflakeConfig.ExistingAccountConfig != nil {
		specifiedEdition = cfg.SnowflakeConfig.ExistingAccountConfig.Edition
	} else {
		sl.ReportError(
			cfg.PantherAccountConfig.Edition,
			"PantherEdition",
			"PantherEdition",
			"eqfield",
			"No Edition appears to have been specified",
		)
	}

	if cfg.PantherAccountConfig.Edition == "ENTERPRISE" && specifiedEdition != "ENTERPRISE" {
		sl.ReportError(
			specifiedEdition,
			"SnowflakeEdition",
			"SnowflakeEdition",
			"eqfield",
			"SnowflakeEdition must be ENTERPRISE if PantherEdition is ENTERPRISE",
		)
	}
}

func validatePantherRegion(fl validator.FieldLevel) bool {
	// TODO: For now, we're hard-coding these. We rarely add new regions,
	// but if we do, we should consider moving this to query a global config
	// service that can answer these sorts of questions.
	validRegions := []string{
		"ap-northeast-1",
		"ap-northeast-2",
		"ap-south-1",
		"ap-southeast-1",
		"ap-southeast-2",
		"ca-central-1",
		"eu-central-1",
		"eu-west-1",
		"eu-west-2",
		"eu-west-3",
		"us-east-1",
		"us-east-2",
		"us-west-2",
	}

	return slices.Contains(validRegions, fl.Field().String())
}

func validateSnowflakeRegion(fl validator.FieldLevel) bool {
	validRegions := []string{
		"aws_ap_northeast_1",
		"aws_ap_northeast_2",
		"aws_ap_south_1",
		"aws_ap_southeast_1",
		"aws_ap_southeast_2",
		"aws_ca_central_1",
		"aws_eu_central_1",
		"aws_eu_west_1",
		"aws_eu_west_2",
		"aws_eu_west_3",
		"aws_us_east_1",
		"aws_us_east_2",
		"aws_us_west_2",
	}

	return slices.Contains(validRegions, fl.Field().String())
}
