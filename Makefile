BIN_DIR := bin
BINARY := sre
PKG := ./cmd/sre
MODULE := github.com/wangqi040318-gif/sre-toolkit

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(MODULE)/internal/version.Version=$(VERSION) \
	-X $(MODULE)/internal/version.Commit=$(COMMIT) \
	-X $(MODULE)/internal/version.Date=$(DATE)

.DEFAULT_GOAL := build

.PHONY: build test fmt vet clean demo check

build:
	mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(PKG)

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

check: fmt vet test

demo: build
	./$(BIN_DIR)/$(BINARY) k8s-pulse --demo

clean:
	rm -rf $(BIN_DIR)
