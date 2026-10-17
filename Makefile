VERSION := $(shell git describe --tags --always)
LDFLAGS := -X github.com/mik3dev/kie-cli/internal/cli.Version=$(VERSION)

.PHONY: build test install lint snapshot tidy-check

build:
	go build -ldflags "$(LDFLAGS)" -o bin/kie-cli ./cmd/kie-cli

test:
	go test -race -cover ./...

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/kie-cli

lint:
	golangci-lint run ./...

snapshot:
	goreleaser release --snapshot --clean

tidy-check:
	go mod tidy -diff
