# Variables
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := "-X 'main.Version=$(GIT_VERSION)' -X 'main.BuildDate=$(BUILD_DATE)'"

# Targets
.PHONY: all server migrate clean test e2e certs tunnel

all: server migrate

server:
	CGO_ENABLED=1 go build -tags='duckdb_arrow' -ldflags=$(LDFLAGS) -o server cmd/server/*.go

migrate:
	CGO_ENABLED=1 go build -tags='duckdb_arrow' -o migrate cmd/migrate/main.go

test:
	CGO_ENABLED=1 go test -tags='duckdb_arrow' ./integration-test/...

e2e:
	bash integration-test/e2e/run.sh

certs:
	mkdir -p .local/certs
	openssl req -x509 -newkey rsa:2048 -keyout .local/certs/server.key \
		-out .local/certs/server.crt -days 365 -nodes \
		-subj "/CN=localhost" \
		-addext "subjectAltName=DNS:localhost,IP:127.0.0.1"

tunnel:
	cloudflared tunnel run --url http://localhost:15000 hugr-dev

clean:
	rm -f server migrate