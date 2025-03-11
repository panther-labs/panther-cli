# GitHub Workflows

This directory contains GitHub Action workflows for automating tasks in this repository.

## Release Workflow

The `release.yml` workflow automatically builds and publishes new releases of the Panther CLI when code is merged to the `main` branch.

### How It Works

1. When code is pushed to the `main` branch, the workflow is triggered.
2. The workflow first runs all tests and linting checks to ensure code quality.
3. If the push doesn't already have a tag, a new version tag is created by:
   - Finding the latest existing tag (or using `v0.0.0` if none exists)
   - Incrementing the patch version (e.g., `v1.2.3` → `v1.2.4`)
   - Creating and pushing a new tag with this version

4. GoReleaser then builds the project according to the configuration in `.goreleaser.yaml`, creating:
   - Binaries for Linux, Windows, and macOS
   - Appropriate archives for each platform
   - A GitHub release with release notes generated from commits

### Manual Triggers

You can also manually trigger a release by creating and pushing a tag with your desired version:

```bash
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
```

### Requirements

For this workflow to function properly, the GitHub Actions runner needs permission to create tags and releases. This is configured in the workflow's `permissions` section.

## Lint, Format, and Test Workflow

The `lint-and-format.yml` workflow automatically checks code linting, formatting, and runs tests on all pull requests.

### How It Works

1. When a pull request is created or updated, the workflow is triggered.
2. The workflow:
   - Sets up a Go environment
   - Installs golangci-lint and gofumpt
   - Runs golangci-lint to check for code quality issues
   - Verifies that all code is correctly formatted with gofumpt
   - Runs all unit tests in the repository

3. If any issues are found (linting errors, formatting issues, or test failures), the workflow will fail and report the problems in the pull request, making them visible to reviewers.

### Benefits

- Ensures consistent code style across the project
- Catches common coding mistakes and anti-patterns early
- Validates functionality through automated tests
- Reduces the burden on code reviewers by automating checks
- Prevents improperly formatted or broken code from being merged

## Running Tests Locally

You can run the same tests locally using the Go test command:

```bash
go test -v ./pkg/...
```