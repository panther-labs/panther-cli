alias b := build
alias bf := build-full
alias bfu := build-full-upgrade
alias bc := build-clean
alias c := clean
alias l := lint
alias rpccs := run-panther-cloud-connected-setup
alias t := test
alias tv := test-verbose
alias tc := test-coverage

build: copy-config
    go build -o ./bin/panther-cloud-connected-setup/panther-cloud-connected-setup ./cmd/panther-cloud-connected-setup/

copy-config:
    #!/usr/bin/env sh
    mkdir -p ./bin/panther-cloud-connected-setup/
    if [ -f "config.yml" ]; then
        echo "Found config.yml file, copying to ./bin/panther-cloud-connected-setup/"
        cp -f config.yml ./bin/panther-cloud-connected-setup/
    fi

build-full: deps lint fmt build

build-full-upgrade: deps-upgrade lint fmt build

deps:
    go get ./...

deps-upgrade:
    go get -u ./...
    go mod tidy

lint:
    golangci-lint run

fmt:
    gofumpt -l -w .

clean:
    rm -rf ./bin/
    rm -rf ./dist/

build-clean: clean build-full-upgrade

run-panther-cloud-connected-setup: build
    ./bin/panther-cloud-connected-setup/panther-cloud-connected-setup --config-file config.yml --verbose

# Run all tests
test:
    go test ./pkg/...

# Run all tests with verbose output
test-verbose:
    go test -v ./pkg/...

# Run tests with coverage report
test-coverage:
    go test -cover ./pkg/...

# Run tests for a specific package (usage: just test-pkg pkg/state)
test-pkg pkg:
    go test -v ./{{pkg}}/...
