# goChess — developer tasks
BINARY      := gochess
PKG         := github.com/cjunius/goChess
BIN_DIR     := bin
CMD         := ./cmd/gochess

VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE        := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

GO          ?= go

.DEFAULT_GOAL := help

## build: compile the binary into ./bin
.PHONY: build
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(CMD)

## install: go install the binary
.PHONY: install
install:
	$(GO) install -trimpath -ldflags "$(LDFLAGS)" $(CMD)

## run: build and start UCI mode
.PHONY: run
run: build
	./$(BIN_DIR)/$(BINARY)

## test: race-enabled unit tests (short)
.PHONY: test
test:
	$(GO) test -race -short ./...

## test-long: full test suite including deep perft
.PHONY: test-long
test-long:
	$(GO) test -race ./...

## bench: run Go benchmarks
.PHONY: bench
bench:
	$(GO) test -run '^$$' -bench . -benchmem ./...

## cover: generate coverage report (coverage.html)
.PHONY: cover
cover:
	$(GO) test -covermode atomic -coverprofile coverage.txt ./...
	$(GO) tool cover -html coverage.txt -o coverage.html
	@echo "wrote coverage.html"

## lint: run golangci-lint
.PHONY: lint
lint:
	golangci-lint run

## fmt: format code
.PHONY: fmt
fmt:
	gofumpt -w .
	goimports -w -local $(PKG) .

## vuln: scan for known vulnerabilities
.PHONY: vuln
vuln:
	govulncheck ./...

## tidy: sync go.mod / go.sum
.PHONY: tidy
tidy:
	$(GO) mod tidy
	$(GO) mod verify

## check: everything CI runs
.PHONY: check
check: tidy lint vuln test

## dev-tools: install development tooling (versions tracked here and in CI)
GOLANGCI_VERSION ?= v2.13.2
.PHONY: dev-tools
dev-tools:
	$(GO) install mvdan.cc/gofumpt@latest
	$(GO) install golang.org/x/tools/cmd/goimports@latest
	$(GO) install golang.org/x/vuln/cmd/govulncheck@v1.1.4
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)

## docker: build the container image
.PHONY: docker
docker:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg DATE=$(DATE) -t $(BINARY):$(VERSION) .

## clean: remove build artifacts
.PHONY: clean
clean:
	rm -rf $(BIN_DIR) dist coverage.txt coverage.html

## help: list targets
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | awk -F ': ' '{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
