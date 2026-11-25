GO_VERSION := $(shell cat .go-version)
LOCAL_GO := $(abspath ../../.runtime/toolchains/go$(GO_VERSION)/bin/go)
GO ?= $(if $(wildcard $(LOCAL_GO)),$(LOCAL_GO),go)
GOFMT = $(shell $(GO) env GOROOT)/bin/gofmt

export GOTOOLCHAIN := local
export GOTELEMETRY := off
export GOCACHE ?= $(abspath ../../.runtime/go-build)
export GOMODCACHE ?= $(abspath ../../.runtime/go-mod)

.PHONY: run build test vet fmt fmt-check check
.PHONY: dev-start dev-stop dev-restart dev-status dev-logs
.PHONY: native-start native-stop native-restart native-status native-logs

run:
	$(GO) run ./cmd/api

build:
	$(GO) build -trimpath -o bin/veilpost-api ./cmd/api

dev-start dev-stop dev-restart dev-status dev-logs:
	@$(GO) run ./cmd/dev-control $(patsubst dev-%,%,$@)

native-start native-stop native-restart native-status native-logs:
	@$(GO) run ./cmd/dev-control --stack $(patsubst native-%,%,$@)

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	$(GOFMT) -w cmd internal tests

fmt-check:
	@test -z "$$($(GOFMT) -l cmd internal tests)" || { $(GOFMT) -l cmd internal tests; exit 1; }

check: fmt-check vet test build
