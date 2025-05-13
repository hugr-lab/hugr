# Variables
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_VERSION := $(shell git describe --tags --always --dirty)
LDFLAGS := "-X 'main.Version=$(GIT_VERSION)' -X 'main.BuildDate=$(BUILD_DATE)'"

# Targets
.PHONY: all management server migrate clean

all: management server migrate

management:
	CGO_ENABLED=1 go build -tags='duckdb_arrow' -ldflags=$(LDFLAGS) -o management cmd/management/

server:
	CGO_ENABLED=1 go build -tags='duckdb_arrow' -ldflags=$(LDFLAGS) -o server cmd/server/*.go

migrate:
	CGO_ENABLED=1 go build -tags='duckdb_arrow' -o migrate cmd/migrate/main.go

clean:
	rm -f management server migrate