# Variables
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := "-X 'main.Version=$(GIT_VERSION)' -X 'main.BuildDate=$(BUILD_DATE)'"

# Targets
.PHONY: all server migrate clean test e2e

all: server migrate

server:
	CGO_ENABLED=1 go build -tags='duckdb_arrow' -ldflags=$(LDFLAGS) -o server cmd/server/*.go

migrate:
	CGO_ENABLED=1 go build -tags='duckdb_arrow' -o migrate cmd/migrate/main.go

test:
	CGO_ENABLED=1 go test -tags='duckdb_arrow' ./integration-test/...

e2e:
	bash integration-test/e2e/run.sh

clean:
	rm -f server migrate