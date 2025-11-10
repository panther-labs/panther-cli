package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSnowflake(t *testing.T) {
	t.Run("returns true when SnowflakeConfig is set", func(t *testing.T) {
		cfg := &Config{
			SnowflakeConfig: &SnowflakeConfig{},
		}
		assert.True(t, cfg.IsSnowflake())
	})

	t.Run("returns false when SnowflakeConfig is nil", func(t *testing.T) {
		cfg := &Config{}
		assert.False(t, cfg.IsSnowflake())
	})
}

func TestIsDatabricks(t *testing.T) {
	t.Run("returns true when DatabricksConfig is set", func(t *testing.T) {
		cfg := &Config{
			DatabricksConfig: &DatabricksConfig{},
		}
		assert.True(t, cfg.IsDatabricks())
	})

	t.Run("returns false when DatabricksConfig is nil", func(t *testing.T) {
		cfg := &Config{}
		assert.False(t, cfg.IsDatabricks())
	})
}

func TestGetDatalakeType(t *testing.T) {
	t.Run("returns Snowflake when SnowflakeConfig is set", func(t *testing.T) {
		cfg := &Config{
			SnowflakeConfig: &SnowflakeConfig{},
		}
		assert.Equal(t, "Snowflake", cfg.GetDatalakeType())
	})

	t.Run("returns Databricks when DatabricksConfig is set", func(t *testing.T) {
		cfg := &Config{
			DatabricksConfig: &DatabricksConfig{},
		}
		assert.Equal(t, "Databricks", cfg.GetDatalakeType())
	})
}
