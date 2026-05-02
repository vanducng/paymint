SHELL := /bin/bash

MODULE  := github.com/vanducng/paymint
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(MODULE)/internal/version.Version=$(VERSION) \
	-X $(MODULE)/internal/version.Commit=$(COMMIT) \
	-X $(MODULE)/internal/version.Date=$(DATE)

.PHONY: build test lint tidy clean install run

build:
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/paymint ./cmd/paymint

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/paymint

test:
	go test ./...

lint:
	golangci-lint run

tidy:
	go mod tidy

clean:
	rm -rf bin/ dist/ coverage.out

run: build
	./bin/paymint $(ARGS)
