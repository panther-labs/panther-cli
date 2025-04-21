package state

import (
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDB(t *testing.T) {
	// Clean up any existing DB file before and after tests
	if err := os.Remove("panther-cli-state.db"); err != nil {
		if !os.IsNotExist(err) {
			log.Fatalf("failed to remove state database: %v\n", err)
		}
	}

	defer func() {
		if err := os.Remove("panther-cli-state.db"); err != nil {
			log.Fatalf("failed to remove state database: %v\n", err)
		}
	}()

	// Test DB creation
	t.Run("Create new DB", func(t *testing.T) {
		db, err := NewDB()
		require.NoError(t, err)
		require.NotNil(t, db)

		// Verify the DB was created
		_, err = os.Stat("panther-cli-state.db")
		assert.NoError(t, err, "Database file should exist")

		// Close the connection
		err = db.Close()
		assert.NoError(t, err)
	})

	// Test state operations
	t.Run("State operations", func(t *testing.T) {
		db, err := NewDB()
		require.NoError(t, err)

		defer func() {
			if err := db.Close(); err != nil {
				log.Fatalf("failed to close database: %v\n", err)
			}
		}()

		// Test getting non-existent state
		configHash := "abcdef1234567890"
		state, err := db.GetState(configHash)
		require.NoError(t, err)
		assert.Nil(t, state, "State should be nil for non-existent config hash")

		// Create a new state
		newState := &Row{
			ConfigHash:                       configHash,
			SnowflakeAccountName:             "testaccount",
			AWSPantherDeploymentRoleDeployed: true,
		}

		// Save the state
		err = db.SaveState(newState)
		require.NoError(t, err)

		// Retrieve the state
		retrievedState, err := db.GetState(configHash)
		require.NoError(t, err)
		require.NotNil(t, retrievedState)
		assert.Equal(t, configHash, retrievedState.ConfigHash)
		assert.Equal(t, "testaccount", retrievedState.SnowflakeAccountName)
		assert.True(t, retrievedState.AWSPantherDeploymentRoleDeployed)

		// Update the state
		newState.SnowflakeAccountName = "updatedaccount"
		err = db.SaveState(newState)
		require.NoError(t, err)

		// Retrieve the updated state
		retrievedState, err = db.GetState(configHash)
		require.NoError(t, err)
		require.NotNil(t, retrievedState)
		assert.Equal(t, "updatedaccount", retrievedState.SnowflakeAccountName)
	})

	// Test multiple configs
	t.Run("Multiple configs", func(t *testing.T) {
		db, err := NewDB()
		require.NoError(t, err)

		defer func() {
			if err := db.Close(); err != nil {
				log.Fatalf("failed to close database: %v\n", err)
			}
		}()

		// Create multiple states with different config hashes
		hash1 := "hash1"
		hash2 := "hash2"

		state1 := &Row{
			ConfigHash:           hash1,
			SnowflakeAccountName: "account1",
		}

		state2 := &Row{
			ConfigHash:           hash2,
			SnowflakeAccountName: "account2",
		}

		// Save both states
		err = db.SaveState(state1)
		require.NoError(t, err)

		err = db.SaveState(state2)
		require.NoError(t, err)

		// Retrieve each state by hash
		retrieved1, err := db.GetState(hash1)
		require.NoError(t, err)
		require.NotNil(t, retrieved1)
		assert.Equal(t, "account1", retrieved1.SnowflakeAccountName)

		retrieved2, err := db.GetState(hash2)
		require.NoError(t, err)
		require.NotNil(t, retrieved2)
		assert.Equal(t, "account2", retrieved2.SnowflakeAccountName)
	})
}
