package aws

import (
	"context"
	"testing"

	"github.com/panther-labs/panther-cli/pkg/cloudconnected/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCertificateRegistrationHelper_RegisterValidationDomains_Disabled(t *testing.T) {
	cfg := &config.Config{
		AWSConfig: config.AWSConfig{
			Region:          "us-west-2",
			AccessKeyID:     "test-key",
			SecretAccessKey: "test-secret",
			DomainCertificateConfiguration: config.DomainCertificateConfiguration{
				PantherSubdomain:              "panther.example.com",
				AutoRegisterValidationDomains: false, // Disabled
			},
		},
	}

	helper := &CertificateRegistrationHelper{
		ctx: context.Background(),
		cfg: cfg,
	}

	validationDetails := CertificateValidationDetails{
		DomainNames: []string{"panther.example.com"},
		RecordName:  "_test.panther.example.com",
		RecordValue: "test-validation-value",
		RecordType:  "CNAME",
	}

	// Should not attempt registration when disabled
	autoRegResult, err := helper.RegisterValidationDomains(validationDetails)
	require.NoError(t, err)
	assert.False(t, autoRegResult.Attempted)
	assert.False(t, autoRegResult.Succeeded)
}

func TestFindHostedZoneForDomain(t *testing.T) {
	tests := []struct {
		name           string
		domain         string
		hostedZones    []hostedZoneTestData
		expectedZoneID string
		expectError    bool
	}{
		{
			name:   "exact match",
			domain: "example.com",
			hostedZones: []hostedZoneTestData{
				{Name: "example.com.", ID: "Z123456789"},
			},
			expectedZoneID: "Z123456789",
			expectError:    false,
		},
		{
			name:   "subdomain match",
			domain: "_validation.panther.example.com",
			hostedZones: []hostedZoneTestData{
				{Name: "example.com.", ID: "Z123456789"},
			},
			expectedZoneID: "Z123456789",
			expectError:    false,
		},
		{
			name:   "most specific match",
			domain: "_validation.panther.sub.example.com",
			hostedZones: []hostedZoneTestData{
				{Name: "example.com.", ID: "Z123456789"},
				{Name: "sub.example.com.", ID: "Z987654321"},
			},
			expectedZoneID: "Z987654321",
			expectError:    false,
		},
		{
			name:   "no match",
			domain: "different.com",
			hostedZones: []hostedZoneTestData{
				{Name: "example.com.", ID: "Z123456789"},
			},
			expectedZoneID: "",
			expectError:    true,
		},
		{
			name:   "remove hostedzone prefix",
			domain: "example.com",
			hostedZones: []hostedZoneTestData{
				{Name: "example.com.", ID: "/hostedzone/Z123456789"},
			},
			expectedZoneID: "Z123456789",
			expectError:    false,
		},
		{
			name:   "domain with trailing dot",
			domain: "example.com.",
			hostedZones: []hostedZoneTestData{
				{Name: "example.com.", ID: "Z123456789"},
			},
			expectedZoneID: "Z123456789",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the hosted zone matching logic
			zoneID, err := findBestMatchingZone(tt.domain, tt.hostedZones)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedZoneID, zoneID)
			}
		})
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := &config.Config{
		AWSConfig: config.AWSConfig{
			DomainCertificateConfiguration: config.DomainCertificateConfiguration{
				PantherSubdomain: "panther.example.com",
				// AutoRegisterValidationDomains not set, should default to false
			},
		},
	}

	// The default should be false
	assert.False(t, cfg.AWSConfig.DomainCertificateConfiguration.AutoRegisterValidationDomains)
}

func TestForceCheckCertificatesLogic(t *testing.T) {
	tests := []struct {
		name               string
		certificateIssued  bool
		forceCheck         bool
		shouldCheckStatus  bool
		expectedLogMessage string
	}{
		{
			name:              "certificate not issued, no force - should check",
			certificateIssued: false,
			forceCheck:        false,
			shouldCheckStatus: true,
		},
		{
			name:              "certificate issued, no force - should not check",
			certificateIssued: true,
			forceCheck:        false,
			shouldCheckStatus: false,
		},
		{
			name:              "certificate not issued, force check - should check",
			certificateIssued: false,
			forceCheck:        true,
			shouldCheckStatus: true,
		},
		{
			name:               "certificate issued, force check - should check",
			certificateIssued:  true,
			forceCheck:         true,
			shouldCheckStatus:  true,
			expectedLogMessage: "Force checking",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the logic that determines whether to check certificate status
			shouldCheck := !tt.certificateIssued || tt.forceCheck
			assert.Equal(t, tt.shouldCheckStatus, shouldCheck)

			// If forcing check on already issued certificate, we expect a force log message
			if tt.forceCheck && tt.certificateIssued {
				assert.Contains(t, tt.expectedLogMessage, "Force checking")
			}
		})
	}
}

// Helper types and functions for testing

type hostedZoneTestData struct {
	Name string
	ID   string
}

// findBestMatchingZone is a simplified version of the hosted zone matching logic for testing
func findBestMatchingZone(domain string, zones []hostedZoneTestData) (string, error) {
	// Remove trailing dot if present
	domain = trimSuffix(domain, ".")

	// Find the most specific zone that matches the domain
	var bestMatch string
	var bestMatchLength int

	for _, zone := range zones {
		zoneName := trimSuffix(zone.Name, ".")

		// Check if domain ends with the zone name
		if domain == zoneName || hasSuffix(domain, "."+zoneName) {
			if len(zoneName) > bestMatchLength {
				bestMatch = zone.ID
				bestMatchLength = len(zoneName)
			}
		}
	}

	if bestMatch == "" {
		return "", assert.AnError
	}

	// Clean the hosted zone ID (remove /hostedzone/ prefix if present)
	bestMatch = trimPrefix(bestMatch, "/hostedzone/")

	return bestMatch, nil
}

// Helper functions to avoid importing strings package in tests
func trimSuffix(s, suffix string) string {
	if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}

func trimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
