package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadinessCheckResults(t *testing.T) {
	// Create a test readiness check result
	results := ReadinessCheckResults{
		DeploymentRoleReadinessResults: []map[string]interface{}{
			{"message": "test error", "severity": "high"},
			{"message": "another issue", "severity": "medium"},
		},
		S3SelectEnabled: true,
	}

	// Test the HasPassed method
	t.Run("HasPassed method", func(t *testing.T) {
		// Should fail because there are items in DeploymentRoleReadinessResults
		assert.False(t, results.HasPassed(), "Should fail with non-empty DeploymentRoleReadinessResults")

		// Empty results should pass, regardless of S3SelectEnabled
		emptyResults := ReadinessCheckResults{
			DeploymentRoleReadinessResults: []map[string]interface{}{},
			S3SelectEnabled:                true,
		}
		assert.True(t, emptyResults.HasPassed(), "Should pass with empty DeploymentRoleReadinessResults")

		// Even with S3Select disabled, empty results should pass according to HasPassed()
		// Note: The Manager.UpdateAWSReadinessState method applies additional criteria with S3SelectEnabled
		s3DisabledResults := ReadinessCheckResults{
			DeploymentRoleReadinessResults: []map[string]interface{}{},
			S3SelectEnabled:                false,
		}
		assert.True(t, s3DisabledResults.HasPassed(), "HasPassed only checks DeploymentRoleReadinessResults")
	})

	// Test JSON serialization/deserialization
	t.Run("JSON serialization", func(t *testing.T) {
		// Serialize to JSON using Value method
		value, err := results.Value()
		require.NoError(t, err)
		require.NotNil(t, value)
		jsonBytes, ok := value.([]byte)
		require.True(t, ok)

		// Deserialize using Scan method
		var deserializedResults ReadinessCheckResults
		err = deserializedResults.Scan(jsonBytes)
		require.NoError(t, err)

		// Verify the deserialized data matches original
		assert.Equal(t, results.S3SelectEnabled, deserializedResults.S3SelectEnabled)
		assert.Len(t, deserializedResults.DeploymentRoleReadinessResults, 2)

		// Check first result
		assert.Equal(t,
			results.DeploymentRoleReadinessResults[0]["message"],
			deserializedResults.DeploymentRoleReadinessResults[0]["message"],
		)

		// Check second result
		assert.Equal(t,
			results.DeploymentRoleReadinessResults[1]["severity"],
			deserializedResults.DeploymentRoleReadinessResults[1]["severity"],
		)
	})

	// Test error case for Scan
	t.Run("Scan error handling", func(t *testing.T) {
		var results ReadinessCheckResults
		err := results.Scan("not-valid-bytes")
		assert.Error(t, err, "Scan should return error for invalid input")
	})
}

func TestCertificateResults(t *testing.T) {
	// Create test certificate results
	results := CertificateResults{
		PantherSubdomain: &CertificateRecord{
			CertificateArn: "arn:aws:acm:region:account:certificate/id",
			ValidationDetails: CertificateValidationRecord{
				DomainNames: []string{"test.example.com"},
				RecordName:  "_validation.test.example.com",
				RecordValue: "validation-value",
				RecordType:  "CNAME",
			},
			IsIssued: true,
		},
	}

	// Test JSON serialization/deserialization
	t.Run("JSON serialization", func(t *testing.T) {
		// Serialize to JSON using Value method
		value, err := results.Value()
		require.NoError(t, err)
		require.NotNil(t, value)
		jsonBytes, ok := value.([]byte)
		require.True(t, ok)

		// Deserialize using Scan method
		var deserializedResults CertificateResults
		err = deserializedResults.Scan(jsonBytes)
		require.NoError(t, err)

		// Verify the deserialized data matches original
		require.NotNil(t, deserializedResults.PantherSubdomain)
		assert.Equal(t, results.PantherSubdomain.CertificateArn, deserializedResults.PantherSubdomain.CertificateArn)
		assert.Equal(
			t,
			results.PantherSubdomain.ValidationDetails.DomainNames,
			deserializedResults.PantherSubdomain.ValidationDetails.DomainNames,
		)
		assert.Equal(
			t,
			results.PantherSubdomain.ValidationDetails.RecordName,
			deserializedResults.PantherSubdomain.ValidationDetails.RecordName,
		)
		assert.Equal(
			t,
			results.PantherSubdomain.ValidationDetails.RecordValue,
			deserializedResults.PantherSubdomain.ValidationDetails.RecordValue,
		)
		assert.Equal(
			t,
			results.PantherSubdomain.ValidationDetails.RecordType,
			deserializedResults.PantherSubdomain.ValidationDetails.RecordType,
		)
		assert.Equal(t, results.PantherSubdomain.IsIssued, deserializedResults.PantherSubdomain.IsIssued)
	})

	// Test nil certificates
	t.Run("Nil certificates", func(t *testing.T) {
		// Create results with nil certificates
		emptyResults := CertificateResults{}

		// Serialize to JSON
		value, err := emptyResults.Value()
		require.NoError(t, err)
		require.NotNil(t, value)
		jsonBytes, ok := value.([]byte)
		require.True(t, ok)

		// Deserialize
		var deserializedResults CertificateResults
		err = deserializedResults.Scan(jsonBytes)
		require.NoError(t, err)

		// Both should be nil
		assert.Nil(t, deserializedResults.PantherSubdomain)
		assert.Nil(t, deserializedResults.WildcardSubdomain)
	})

	// Test error case for Scan
	t.Run("Scan error handling", func(t *testing.T) {
		var results CertificateResults
		err := results.Scan("not-valid-bytes")
		assert.Error(t, err, "Scan should return error for invalid input")

		// Test with nil input
		err = results.Scan(nil)
		assert.Error(t, err, "Scan should return error for nil input")
	})
}
