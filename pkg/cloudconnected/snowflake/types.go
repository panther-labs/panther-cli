package snowflake

import (
	"crypto/rsa"
	"strings"

	"github.com/pkg/errors"
)

type CreateAccountResult struct {
	AccountLocator    string `json:"accountLocator"`
	AccountLocatorURL string `json:"accountLocatorURL"`
	AccountName       string `json:"accountName"`
	URL               string `json:"url"`
	Edition           string `json:"edition"`
	RegionGroup       string `json:"regionGroup"`
	Cloud             string `json:"cloud"`
	Region            string `json:"region"`
	AdminRSAKey       *rsa.PrivateKey
}

func (c CreateAccountResult) GetAWSRegion() string {
	lowered := strings.ToLower(c.Region)
	stripped := strings.TrimPrefix(lowered, "aws_")
	toDashes := strings.ReplaceAll(stripped, "_", "-")
	return toDashes
}

// GetFullyQualifiedAccountName extracts the first subdomain from the URL
// Example: for URL "https://pantherlabs-zbrown08test.snowflakecomputing.com"
// it returns "pantherlabs-zbrown08test". This is needed because while the
// URL that Snowflake returns on account creation has a fully qualified account
// name, the "AccountName" field is not fully qualified.
//
// In this context, "fully qualified" means the first subdomain is in the format
// "<org name>-<account name>". The AccountName returned by Snowflake is just
// "<account name>".
func (c CreateAccountResult) GetFullyQualifiedAccountName() (string, error) {
	// Remove protocol prefix if present
	urlParts := strings.Split(c.URL, "://")
	hostPart := c.URL
	if len(urlParts) > 1 {
		hostPart = urlParts[1]
	}

	// Split by dots and take first part
	parts := strings.Split(hostPart, ".")
	if len(parts) > 0 {
		return parts[0], nil
	}
	return "", errors.Errorf(
		"URL does not contain a subdomain, the URL is not valid. Is the CreateAccountResult well formed? -- %+v",
		c,
	)
}
