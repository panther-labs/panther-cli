alias b := build
alias bf := build-full
alias bc := build-clean
alias c := clean
alias l := lint
alias rpccs := run-panther-cloud-connected-setup

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

deps:
    go get -u ./...
    go mod tidy

lint:
    golangci-lint run

fmt:
    gofumpt -l -w .

clean:
    rm -rf ./bin/
    rm -rf ./dist/

build-clean: clean build-full

run-panther-cloud-connected-setup: build
    ./bin/panther-cloud-connected-setup/panther-cloud-connected-setup --config-file config.yml --verbose
