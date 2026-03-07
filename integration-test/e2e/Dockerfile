FROM golang:1.26 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_CFLAGS="-O1 -g" go build -tags=duckdb_arrow -ldflags="-X 'main.Version=e2e-test' -X 'main.BuildDate=2026-03-07'" -o /server ./cmd/server

FROM debian:trixie-slim
RUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*
COPY --from=builder /server /usr/local/bin/server
ENTRYPOINT ["server"]
