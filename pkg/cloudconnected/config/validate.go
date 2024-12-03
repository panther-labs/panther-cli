package config

import (
	"regexp"

	"github.com/go-playground/validator/v10"
	"github.com/panther-labs/panther-cli/pkg/util"
)

var validate *validator.Validate = validator.New(validator.WithRequiredStructEnabled())

func init() {
	util.Must(
		validate.RegisterValidation("validAcctName", validateAccountName),
		"couldn't register validAcctName validation",
	)
	util.Must(validate.RegisterValidation("validAdminName", validateAdminName),
		"couldn't register validAdminName validation",
	)
	util.Must(validate.RegisterValidation("validPantherRegion", validatePantherRegion),
		"couldn't register validPantherRegion validation",
	)

	validate.RegisterStructValidation(validateEditionsMatch, NewAccountConfig{})
}

func validateAccountName(fl validator.FieldLevel) bool {
	accountNameRegex := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	return accountNameRegex.MatchString(fl.Field().String())
}

func validateAdminName(fl validator.FieldLevel) bool {
	adminNameRegex := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	return adminNameRegex.MatchString(fl.Field().String())
}

func validateEditionsMatch(sl validator.StructLevel) {
	cfg := sl.Current().Interface().(NewAccountConfig)
	if cfg.PantherEdition == "ENTERPRISE" && cfg.SnowflakeEdition != "ENTERPRISE" {
		sl.ReportError(
			cfg.SnowflakeEdition,
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

	for _, region := range validRegions {
		if fl.Field().String() == region {
			return true
		}
	}
	return false
}
