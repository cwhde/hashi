# The only entry point for build, check, test and deploy. `make help` lists targets.

SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

TOOL      := go tool -modfile=tools/go.mod
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS   := -s -w -X git.juzo.io/juzo/hashi/internal/app.Version=$(VERSION) -X git.juzo.io/juzo/hashi/internal/app.Commit=$(COMMIT)

.PHONY: help check fmt-check lint test test-race generate generate-check build clean

help: ## list targets
	@awk 'BEGIN{FS=":.*## "} /^[a-z-]+:.*## /{printf "  %-18s %s\n",$$1,$$2}' $(MAKEFILE_LIST)

check: fmt-check lint test generate-check ## everything CI runs on every change (must stay < 3 min)

fmt-check: ## fail if any Go file is not gofmt-ed
	@out="$$(gofmt -l $$(git ls-files -co --exclude-standard '*.go'))"; \
	  if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

lint: ## go vet + golangci-lint (pinned in tools/go.mod)
	go vet ./...
	$(TOOL) golangci-lint run ./...

test: ## unit + contract tests
	go test -race -count=1 ./...

generate: ## regenerate sqlc and OpenAPI code
	@if [ -f sqlc.yaml ]; then $(TOOL) sqlc generate; fi
	$(TOOL) oapi-codegen -config internal/httpapi/gen/oapi.yaml api/openapi.yaml

GENERATED := $(wildcard internal/store/db internal/httpapi/gen/server.gen.go)

generate-check: ## fail if generated code is stale (compares checksums before/after regenerating)
	@[ -n "$(GENERATED)" ] || exit 0; \
	  before="$$(find $(GENERATED) -type f -print0 | sort -z | xargs -0 cksum | cksum)"; \
	  $(MAKE) --no-print-directory generate >/dev/null; \
	  after="$$(find $(GENERATED) -type f -print0 | sort -z | xargs -0 cksum | cksum)"; \
	  if [ "$$before" != "$$after" ]; then echo "generated code was stale: regenerated, review and commit it"; exit 1; fi

build: ## build bin/hashi for this machine
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/hashi ./cmd/hashi

clean:
	rm -rf bin dist
