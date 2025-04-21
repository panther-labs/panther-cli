package snowflake

import "github.com/go-playground/validator/v10"

var validate *validator.Validate = validator.New(validator.WithRequiredStructEnabled())
