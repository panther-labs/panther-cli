package snowflake

import (
	"context"
	"log"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/rsapem"
	"github.com/pkg/errors"
)

type SnowflakeSetup struct {
	ctx context.Context
	cfg *config.Config
}

func NewSnowflakeSetup(ctx context.Context, cfg *config.Config) *SnowflakeSetup {
	return &SnowflakeSetup{ctx, cfg}
}

func (s *SnowflakeSetup) CreateOrResolveAccount() (resolvedAccount *ResolvedSnowflakeAcccount, err error) {
	if s.cfg.SnowflakeConfig.ConfigType == config.SnowflakeConfigTypeNewAccount {
		log.Println("Creating new Snowflake account")
		resolvedAccount, err = s.createSnowflakeAccount()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create Snowflake account")
		}
	} else {
		log.Println("Existing Snowflake account specified, resolving account details")

		// fill out createAccountResult with the existing account details
		privateKeyAsStr, err := s.cfg.SnowflakeConfig.ExistingAccountConfig.LoadPantherAccountAdminRSAKey()
		if err != nil {
			return nil, errors.Wrap(err, "failed to load Panther account admin RSA key")
		}

		privateKey, err := rsapem.ParseRSAPEMPrivateKey(privateKeyAsStr)
		if err != nil {
			return nil, errors.Wrap(err, "failed to decode Panther account admin RSA key")
		}

		resolvedAccount = &ResolvedSnowflakeAcccount{
			AccountName: s.cfg.SnowflakeConfig.ExistingAccountConfig.AccountName,
			URL:         s.cfg.SnowflakeConfig.ExistingAccountConfig.URL,
			Edition:     s.cfg.SnowflakeConfig.ExistingAccountConfig.Edition,
			Region:      s.cfg.SnowflakeConfig.ExistingAccountConfig.Region,
			AdminRSAKey: privateKey,
		}
	}

	if err := validate.Struct(resolvedAccount); err != nil {
		return nil, errors.Wrap(err, "failed to validate resolved Snowflake account")
	}

	return resolvedAccount, nil
}

func (s *SnowflakeSetup) SetupAccount(resolvedAccount *ResolvedSnowflakeAcccount) error {
	if s.cfg.SnowflakeConfig.ConfigType == config.SnowflakeConfigTypeNewAccount {
		log.Println("Setting up new Snowflake account's admin user")
		if err := s.setupSnowflakeAdmin(resolvedAccount); err != nil {
			return errors.Wrap(err, "failed to setup Snowflake account")
		}
	}

	return nil
}

// Uses orgadmin (provided with RSA key) to create a new account whose first admin user is
// PANTHERACCOUNTADMIN with a newly generated RSA key.
func (s *SnowflakeSetup) createSnowflakeAccount() (*ResolvedSnowflakeAcccount, error) {
	snow := AccountCreate{}

	if err := snow.Connect(s.ctx, s.cfg.SnowflakeConfig.NewAccountConfig.OrgConfig); err != nil {
		return nil, errors.Wrap(err, "failed to connect to Snowflake")
	}
	defer func() {
		if err := snow.Close(); err != nil {
			log.Fatalf("failed to close Snowflake connection: %v\n", err)
		}
	}()

	createAcctRes, err := snow.CreateNewSnowflakeAccount(s.cfg.SnowflakeConfig.NewAccountConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new Snowflake account")
	}

	return createAcctRes, nil
}

// Creates a type=person admin user for the customer based on the provided config.
func (s *SnowflakeSetup) setupSnowflakeAdmin(
	createAcctRes *ResolvedSnowflakeAcccount,
) error {
	snowAcctSetup := AccountSetup{}
	if err := snowAcctSetup.Connect(s.ctx, createAcctRes); err != nil {
		return errors.Wrap(err, "failed to connect to new Snowflake account")
	}
	defer func() {
		if err := snowAcctSetup.Close(); err != nil {
			log.Fatalf("failed to close Snowflake account setup: %v\n", err)
		}
	}()

	if err := snowAcctSetup.SetupCustomerAccountAdminUser(s.cfg.SnowflakeConfig.NewAccountConfig); err != nil {
		return errors.Wrap(err, "failed to setup Panther account admin user")
	}
	return nil
}
