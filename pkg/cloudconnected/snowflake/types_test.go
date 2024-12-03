package snowflake

import "testing"

func TestGetFullyQualifiedAccountName(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "URL with protocol",
			url:      "https://abc123.us-east-1.snowflakecomputing.com",
			expected: "abc123",
		},
		{
			name:     "URL without protocol",
			url:      "xyz789.us-west-2.snowflakecomputing.com",
			expected: "xyz789",
		},
		{
			name:     "Empty URL",
			url:      "",
			expected: "",
		},
		{
			name:     "URL with no subdomain",
			url:      "snowflakecomputing.com",
			expected: "snowflakecomputing",
		},
		{
			name:     "URL with multiple subdomains",
			url:      "test.abc123.us-east-1.snowflakecomputing.com",
			expected: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CreateAccountResult{URL: tt.url}
			got, _ := result.GetFullyQualifiedAccountName()
			if got != tt.expected {
				t.Errorf("GetFullyQualifiedAccountName() = %v, want %v", got, tt.expected)
			}
		})
	}
}
