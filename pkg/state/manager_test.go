package state

import (
	"crypto/rand"
	"crypto/rsa"
	"log"
	"os"
	"testing"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/aws"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/panther-labs/panther-cli/pkg/cloudconnected/snowflake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	// Clean up the database file after tests
	defer func() {
		if err := os.Remove("panther-cli-state.db"); err != nil {
			if !os.IsNotExist(err) {
				log.Fatalf("failed to remove state database: %v\n", err)
			}
		}
	}()

	// Create a simple test config
	cfg := &config.Config{
		AWSConfig: config.AWSConfig{
			AccessKeyID:     "test-access-key",
			SecretAccessKey: "test-secret-key",
			Region:          "us-west-2",
			CloudFormationConfig: config.CloudFormationConfig{
				IdentityAccountId: "123456789012",
				OpsAccountId:      "210987654321",
			},
			DomainCertificateConfiguration: config.DomainCertificateConfiguration{
				PantherSubdomain: "test.example.com",
			},
		},
	}

	// Test creating a new manager
	t.Run("Create new manager", func(t *testing.T) {
		manager, err := NewManager(cfg)
		require.NoError(t, err)
		require.NotNil(t, manager)

		// Verify the manager has a non-empty config hash
		assert.NotEmpty(t, manager.configHash)

		// Verify the state is initialized with the config hash
		state := manager.GetState()
		assert.Equal(t, manager.configHash, state.ConfigHash)

		// Clean up
		err = manager.Close()
		assert.NoError(t, err)
	})

	// Test retrieving the same state with the same config
	t.Run("Retrieve existing state", func(t *testing.T) {
		// Create first manager
		manager1, err := NewManager(cfg)
		require.NoError(t, err)
		configHash1 := manager1.configHash

		// Create another manager with the same config
		manager2, err := NewManager(cfg)
		require.NoError(t, err)

		// Verify the config hashes match
		assert.Equal(t, configHash1, manager2.configHash)

		// Clean up
		_ = manager1.Close()
		_ = manager2.Close()
	})

	// Test saving and retrieving state changes
	t.Run("Save and retrieve state", func(t *testing.T) {
		manager, err := NewManager(cfg)
		require.NoError(t, err)

		// Update state
		err = manager.UpdateAWSDeploymentState(true)
		require.NoError(t, err)

		// Close and reopen the manager
		_ = manager.Close()

		// New manager should retrieve the updated state
		newManager, err := NewManager(cfg)
		require.NoError(t, err)

		// Verify the state was saved and retrieved
		state := newManager.GetState()
		assert.True(t, state.AWSPantherDeploymentRoleDeployed)

		// Clean up
		_ = newManager.Close()
	})
}

func TestStateUpdates(t *testing.T) {
	// Clean up the database file after tests
	defer func() {
		if err := os.Remove("panther-cli-state.db"); err != nil {
			if !os.IsNotExist(err) {
				log.Fatalf("failed to remove state database: %v\n", err)
			}
		}
	}()

	// Create a simple test config
	cfg := &config.Config{
		AWSConfig: config.AWSConfig{
			AccessKeyID:     "test-access-key",
			SecretAccessKey: "test-secret-key",
			Region:          "us-west-2",
			CloudFormationConfig: config.CloudFormationConfig{
				IdentityAccountId: "123456789012",
				OpsAccountId:      "210987654321",
			},
			DomainCertificateConfiguration: config.DomainCertificateConfiguration{
				PantherSubdomain: "test.example.com",
			},
		},
	}

	// Test UpdateAWSBootstrapState
	t.Run("UpdateAWSBootstrapState", func(t *testing.T) {
		manager, err := NewManager(cfg)
		require.NoError(t, err)
		defer func() {
			if err := manager.Close(); err != nil {
				log.Fatalf("failed to close manager: %v\n", err)
			}
		}()

		// Update bootstrap state
		err = manager.UpdateAWSBootstrapState(true)
		require.NoError(t, err)

		// Verify state was updated
		state := manager.GetState()
		assert.True(t, state.AWSReadinessBootstrapToolsDeployed)

		// Update to false
		err = manager.UpdateAWSBootstrapState(false)
		require.NoError(t, err)

		// Verify state was updated
		state = manager.GetState()
		assert.False(t, state.AWSReadinessBootstrapToolsDeployed)
	})

	// Test UpdateAWSReadinessState
	t.Run("UpdateAWSReadinessState", func(t *testing.T) {
		manager, err := NewManager(cfg)
		require.NoError(t, err)
		defer func() {
			if err := manager.Close(); err != nil {
				log.Fatalf("failed to close manager: %v\n", err)
			}
		}()

		// Create test readiness results - empty array for success
		results := ReadinessCheckResults{
			DeploymentRoleReadinessResults: []map[string]interface{}{},
			S3SelectEnabled:                true,
		}

		// Update readiness state
		err = manager.UpdateAWSReadinessState(results)
		require.NoError(t, err)

		// Verify state
		state := manager.GetState()
		assert.Equal(t, results, state.AWSReadinessCheckResults)
		assert.True(t, state.AWSReadinessCheckSucceeded) // Empty deployment results = success

		// Test with failure condition - array with errors
		results = ReadinessCheckResults{
			DeploymentRoleReadinessResults: []map[string]interface{}{
				{"error": "Error 1"},
				{"error": "Error 2"},
			},
			S3SelectEnabled: true,
		}

		err = manager.UpdateAWSReadinessState(results)
		require.NoError(t, err)

		// Verify state - should now be failed
		state = manager.GetState()
		assert.Equal(t, results, state.AWSReadinessCheckResults)
		assert.False(t, state.AWSReadinessCheckSucceeded) // Non-empty deployment results = failure

		// Test with S3 select disabled - empty errors should still succeed
		results = ReadinessCheckResults{
			DeploymentRoleReadinessResults: []map[string]interface{}{},
			S3SelectEnabled:                false,
		}

		err = manager.UpdateAWSReadinessState(results)
		require.NoError(t, err)

		// Verify state - should succeed because S3SelectEnabled is not a criteria
		state = manager.GetState()
		assert.Equal(t, results, state.AWSReadinessCheckResults)
		assert.True(t, state.AWSReadinessCheckSucceeded) // Should be true since S3SelectEnabled doesn't matter
	})

	// Test UpdateAWSSnowflakeBootstrapState
	t.Run("UpdateAWSSnowflakeBootstrapState", func(t *testing.T) {
		manager, err := NewManager(cfg)
		require.NoError(t, err)
		defer func() {
			if err := manager.Close(); err != nil {
				log.Fatalf("failed to close manager: %v\n", err)
			}
		}()

		// Update Snowflake bootstrap state
		testARN := "arn:aws:secretsmanager:us-west-2:123456789012:secret:test-secret-123456"
		err = manager.UpdateAWSSnowflakeBootstrapState(true, testARN)
		require.NoError(t, err)

		// Verify state
		state := manager.GetState()
		assert.True(t, state.AWSSnowflakeBootstrapSucceeded)
		assert.Equal(t, testARN, state.AWSSnowflakeSecretARN)

		// Update to false
		err = manager.UpdateAWSSnowflakeBootstrapState(false, "")
		require.NoError(t, err)

		// Verify state
		state = manager.GetState()
		assert.False(t, state.AWSSnowflakeBootstrapSucceeded)
		assert.Empty(t, state.AWSSnowflakeSecretARN)
	})

	// Test UpdateCertificateState
	t.Run("UpdateCertificateState", func(t *testing.T) {
		manager, err := NewManager(cfg)
		require.NoError(t, err)
		defer func() {
			if err := manager.Close(); err != nil {
				log.Fatalf("failed to close manager: %v\n", err)
			}
		}()

		// Create test certificate data
		certResult := aws.CertificateRegistrationResult{
			CertificateArn: "arn:aws:acm:us-west-2:123456789012:certificate/1234-5678-9012",
			ValidationDetails: aws.CertificateValidationDetails{
				DomainName:  "test.example.com",
				RecordName:  "_1234.test.example.com",
				RecordValue: "verification-value-1234",
				RecordType:  "CNAME",
			},
		}

		// Update panther certificate state
		err = manager.UpdateCertificateState("panther", certResult, false)
		require.NoError(t, err)

		// Verify state
		state := manager.GetState()
		assert.True(t, state.AWSCertificatesRequested)
		assert.Equal(t, certResult.CertificateArn, state.AWSCertificatesResults.PantherSubdomain.CertificateArn)
		assert.Equal(
			t,
			certResult.ValidationDetails.DomainName,
			state.AWSCertificatesResults.PantherSubdomain.ValidationDetails.DomainName,
		)
		assert.Equal(
			t,
			certResult.ValidationDetails.RecordName,
			state.AWSCertificatesResults.PantherSubdomain.ValidationDetails.RecordName,
		)
		assert.Equal(
			t,
			certResult.ValidationDetails.RecordValue,
			state.AWSCertificatesResults.PantherSubdomain.ValidationDetails.RecordValue,
		)
		assert.Equal(
			t,
			certResult.ValidationDetails.RecordType,
			state.AWSCertificatesResults.PantherSubdomain.ValidationDetails.RecordType,
		)
		assert.False(t, state.AWSCertificatesResults.PantherSubdomain.IsIssued)

		// Create wildcard certificate data
		wildcardResult := aws.CertificateRegistrationResult{
			CertificateArn: "arn:aws:acm:us-west-2:123456789012:certificate/9876-5432-1098",
			ValidationDetails: aws.CertificateValidationDetails{
				DomainName:  "*.example.com",
				RecordName:  "_9876.example.com",
				RecordValue: "verification-value-9876",
				RecordType:  "CNAME",
			},
		}

		// Update wildcard certificate state
		err = manager.UpdateCertificateState("wildcard", wildcardResult, true)
		require.NoError(t, err)

		// Verify both certificates are in state
		state = manager.GetState()
		assert.Equal(t, wildcardResult.CertificateArn, state.AWSCertificatesResults.WildcardSubdomain.CertificateArn)
		assert.Equal(
			t,
			wildcardResult.ValidationDetails.DomainName,
			state.AWSCertificatesResults.WildcardSubdomain.ValidationDetails.DomainName,
		)
		assert.True(t, state.AWSCertificatesResults.WildcardSubdomain.IsIssued)

		// Panther cert should still be there
		assert.Equal(t, certResult.CertificateArn, state.AWSCertificatesResults.PantherSubdomain.CertificateArn)

		// Test invalid certificate type
		err = manager.UpdateCertificateState("invalid", certResult, true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid certificate type")
	})
}

func TestSnowflakeState(t *testing.T) {
	// Clean up the database file after tests
	defer func() {
		if err := os.Remove("panther-cli-state.db"); err != nil {
			if !os.IsNotExist(err) {
				log.Fatalf("failed to remove state database: %v\n", err)
			}
		}
	}()

	// Create a simple test config
	cfg := &config.Config{
		AWSConfig: config.AWSConfig{
			AccessKeyID:     "test-access-key",
			SecretAccessKey: "test-secret-key",
			Region:          "us-west-2",
		},
	}

	t.Run("UpdateSnowflakeState", func(t *testing.T) {
		manager, err := NewManager(cfg)
		require.NoError(t, err)
		defer func() {
			if err := manager.Close(); err != nil {
				log.Fatalf("failed to close manager: %v\n", err)
			}
		}()

		// Create a proper RSA key for testing
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		require.NotNil(t, privateKey)

		// Create mock Snowflake account result
		accountDetails := &snowflake.ResolvedSnowflakeAcccount{
			AccountName: "panther_test-account123",
			Region:      "aws_us_west_2",
			Edition:     "ENTERPRISE",
			URL:         "https://panther_test-account123.snowflakecomputing.com",
			AdminRSAKey: privateKey,
		}

		// Update Snowflake state
		err = manager.UpdateSnowflakeState(accountDetails, true)
		require.NoError(t, err)

		// Verify state
		state := manager.GetState()
		assert.Equal(t, accountDetails.AccountName, state.SnowflakeAccountName)
		assert.Equal(t, accountDetails.Region, state.SnowflakeRegion)
		assert.Equal(t, accountDetails.Edition, state.SnowflakeEdition)
		assert.Equal(t, accountDetails.URL, state.SnowflakeAccountURL)
	})
}
