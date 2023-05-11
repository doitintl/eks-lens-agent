# Go parameters
GO  = go
BIN = $(CURDIR)/.bin
LINT_CONFIG = $(CURDIR)/.golangci.yaml

MODULE   = $(shell $(GO) list -m)
DATE    ?= $(shell date +%FT%T%z)
VERSION ?= $(shell git describe --tags --always --dirty --match="[0-9]*.[0-9]*.[0-9]*" 2> /dev/null || \
			cat $(CURDIR)/.version 2> /dev/null || echo v0)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null)
BRANCH  ?= $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)

LDFLAGS_VERSION := -X main.version=$(VERSION) -X main.gitCommit=$(COMMIT) -X main.gitBranch=$(BRANCH) -X \"main.buildDate=$(DATE)\"

# platforms and architectures for release; default to MacOS (darwin) and arm64 (M1/M2)
TARGETOS   := $(or $(TARGETOS), darwin)
TARGETARCH := $(or $(TARGETARCH), arm64)
PLATFORMS     = darwin linux windows
ARCHITECTURES = amd64 arm64

TIMEOUT = 15
V = 0
Q = $(if $(filter 1,$V),,@)
M = $(shell printf "\033[34;1m▶\033[0m")

export CGO_ENABLED=0
export GOPROXY=https://proxy.golang.org
export GOOS=$(TARGETOS)
export GOARCH=$(TARGETARCH)

.PHONY: all
all: fmt lint test build

.PHONY: dependency
dependency: ; $(info $(M) downloading dependencies...) @ ## Build program binary
	$Q $(GO) mod download

.PHONY: build
build: dependency | ; $(info $(M) building $(GOOS)/$(GOARCH) binary...) @ ## Build program binary
	$Q env GOOS=$(TARGETOS) GOARCH=$(TARGETARCH) $(GO) build \
		-tags release \
		-ldflags "$(LDFLAGS_VERSION)" \
		-o $(BIN)/$(basename $(MODULE)) ./cmd/main.go

.PHONY: release
release: clean ; $(info $(M) building binaries for multiple os/arch...) @ ## Build program binary for paltforms and os
	$(foreach GOOS, $(PLATFORMS),\
		$(foreach GOARCH, $(ARCHITECTURES), \
			$(shell \
				GOPROXY=$(GOPROXY) CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
				$(GO) build \
				-tags release \
				-ldflags "$(LDFLAGS_VERSION)" \
				-o $(BIN)/$(basename $(MODULE))_$(GOOS)_$(GOARCH) ./cmd/main.go || true)))

# Tools

setup-tools: setup-lint setup-gomock

setup-lint:
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.51.2
setup-gomock:
	$(GO) install github.com/golang/mock/mockgen@v1.6.0

GOLINT=golangci-lint
GOMOCK=gomock

# Tests

.PHONY: test
test: ; $(info $(M) running test ...) @ ## run tests with coverage
	$Q $(GO) test -v -cover ./... -coverprofile=coverage.out
	$Q $(GO) tool cover -func=coverage.out

.PHONY: test-json
test-json: ; $(info $(M) running test output JSON ...) @ ## run tests with JSON report and coverage
	$Q $(GO) test -v -cover ./... -coverprofile=coverage.out -json > test-report.out
	$Q $(GO) tool cover -func=coverage.out

.PHONY: test-view
test-view: ; $(info $(M) generating coverage report ...) @ ## generate HTML coverage report
	$(GO) tool cover -html=coverage.out

.PHONY: fmt
fmt: ; $(info $(M) running gofmt...) @ ## Run gofmt on all source files
	$Q $(GO) fmt ./...

.PHONY: lint
lint: setup-lint; $(info $(M) running golangci-lint...) @ ## Run golangci-lint
	$Q $(GOLINT) run -v -c $(LINT_CONFIG) ./...

# generate test mock for interfaces
.PHONY: mockgen
mockgen: setup-gomock ; $(info $(M) generating mocks...) @ ## Run mockery
	$Q $(GO) generate ./...

# Misc

.PHONY: clean
clean: ; $(info $(M) cleaning...)	@ ## Cleanup everything
	@rm -rf $(BIN)
	@rm -rf test/tests.* test/coverage.*

.PHONY: help
help:
	@grep -E '^[ a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.PHONY: version
version:
	@echo $(VERSION)
