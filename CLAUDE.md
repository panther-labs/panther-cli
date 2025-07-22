# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go CLI toolset for Panther Labs that provides tooling for administering and provisioning Panther instances. The primary tool is `panther-cloud-connected-setup`, which automates the initial provisioning steps for Snowflake and AWS for new Cloud Connected Deployments.

## Key Architecture Components

- **State Management**: Uses SQLite database (`panther-cli-state.db`) to track provisioning progress across runs via `pkg/state`
- **Configuration**: YAML-based config with JSON schema validation in `pkg/cloudconnected/config`
- **Cloud Providers**: Separate packages for AWS (`pkg/cloudconnected/aws`) and Snowflake (`pkg/cloudconnected/snowflake`) operations
- **Utilities**: Common utilities in `pkg/util` for logging, HTTP operations, and cloud provider helpers

## Build and Development Commands

This project uses `just` (justfile) instead of Makefile for task automation:

### Primary Commands
- `just build` or `just b` - Build the main binary with CGO disabled
- `just build-full` or `just bf` - Full build with deps, lint, fmt, and build
- `just build-full-upgrade` or `just bfu` - Upgrade deps then full build
- `just lint` or `just l` - Run golangci-lint with fixes
- `just clean` or `just c` - Remove build artifacts

### Testing Commands  
- `just test` or `just t` - Run all tests in pkg/
- `just test-verbose` or `just tv` - Run tests with verbose output
- `just test-coverage` or `just tc` - Run tests with coverage report
- `just test-pkg <pkg>` - Run tests for specific package (e.g., `just test-pkg pkg/state`)

### Development Commands
- `just run-panther-cloud-connected-setup` or `just rpccs` - Build and run main tool with config.yml
- `just deps` - Get dependencies
- `just fmt` - Format code via golangci-lint

### Release Commands
- `just build-release` - Build release artifacts with goreleaser

## Code Style Guidelines

The project follows strict Go coding standards enforced via Cursor rules:

### Error Handling
- Use `github.com/pkg/errors` for error wrapping with context
- Use `errors.Wrapf` instead of `errors.Wrap(err, fmt.Sprintf(...))`
- Always handle errors explicitly
- Return errors as last return value

### Testing
- Use `github.com/stretchr/testify` for assertions
- Use table-driven test patterns
- Use `require.ErrorContains()` instead of redundant `require.Error() + require.ErrorContains()`

### Logging
- Use `util.LogWarnf()` and `util.LogWarnln()` for warning messages instead of `log.Printf("Warning: ...")` or `log.Println("Warning: ...")`
- Use `util.LogDebugf()` and `util.LogDebugln()` for debug output (controlled by DEBUG environment variable)
- Use standard `log.Printf()` and `log.Println()` for normal informational output

### Concurrency
- Follow Dave Cheney's concurrency recommendations
- Never start goroutines without knowing when they'll stop
- Use context for cancellation
- Close channels only from sender
- Use errgroup for concurrent error handling

## Important Configuration Notes

- Config files are YAML with JSON schema validation (`pkg/cloudconnected/config/schema.yaml`)
- Supports both Snowflake and Redshift as data lake backends
- RSA keypair authentication required for Snowflake
- State is persisted in SQLite database for resumable operations
- Sensitive data stored in state file should be cleaned with `--clean` flag after successful runs

## Main Entry Points

- `cmd/panther-cloud-connected-setup/main.go` - Primary CLI tool
- `cmd/pubkey-from-privkey-pem/main.go` - Utility for RSA key operations

## Package Structure

- `pkg/cloudconnected/` - Core provisioning logic
- `pkg/state/` - SQLite-based state management
- `pkg/rsapem/` - RSA key operations
- `pkg/util/` - Common utilities (logging, HTTP, cloud helpers)