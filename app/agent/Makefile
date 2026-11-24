GO_VERSION := $(shell cat ../backend/.go-version)
LOCAL_GO := $(abspath ../../.runtime/toolchains/go$(GO_VERSION)/bin/go)
GO := $(if $(wildcard $(LOCAL_GO)),$(LOCAL_GO),go)
GOFMT = $(shell $(GO) env GOROOT)/bin/gofmt

export GOTOOLCHAIN := local
export GOTELEMETRY := off
export GOCACHE ?= $(abspath ../../.runtime/go-build)
export GOMODCACHE ?= $(abspath ../../.runtime/go-mod)

.PHONY: build test vet fmt fmt-check check

build:
	CGO_ENABLED=0 $(GO) build -trimpath -o bin/veilpost-agent ./cmd/veilpost-agent

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	$(GOFMT) -w cmd internal tests

fmt-check:
	@test -z "$$($(GOFMT) -l cmd internal tests)" || { $(GOFMT) -l cmd internal tests; exit 1; }

check: fmt-check vet test build
