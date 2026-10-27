# The only entry point for build, check, test and deploy. `make help` lists targets.

SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

TOOL      := go tool -modfile=tools/go.mod
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS   := -s -w -X git.juzo.io/juzo/hashi/internal/app.Version=$(VERSION) -X git.juzo.io/juzo/hashi/internal/app.Commit=$(COMMIT)

.PHONY: build-config-check help image deploy rollback smoke check fmt-check lint test generate generate-check web-install web-check build-web build dev clean

help: ## list targets
	@awk 'BEGIN{FS=":.*## "} /^[a-z-]+:.*## /{printf "  %-18s %s\n",$$1,$$2}' $(MAKEFILE_LIST)

check: fmt-check lint test generate-check web-check build-config-check ## everything CI runs on every change (must stay < 3 min)

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
	cd web && pnpm run --silent generate:api

GENERATED := $(wildcard internal/store/db internal/httpapi/gen/server.gen.go web/src/lib/api/schema.gen.ts)

generate-check: ## fail if generated code is stale (compares checksums before/after regenerating)
	@[ -n "$(GENERATED)" ] || exit 0; \
	  before="$$(find $(GENERATED) -type f -print0 | sort -z | xargs -0 cksum | cksum)"; \
	  $(MAKE) --no-print-directory generate >/dev/null; \
	  after="$$(find $(GENERATED) -type f -print0 | sort -z | xargs -0 cksum | cksum)"; \
	  if [ "$$before" != "$$after" ]; then echo "generated code was stale: regenerated, review and commit it"; exit 1; fi

build-config-check: ## Dockerfile cross-compiles on $$BUILDPLATFORM; release keeps amd64+arm64 (11-delivery §6.2)
	@f=deploy/docker/Dockerfile; w=.gitea/workflows/release.yml; ok=1; \
	  [ "$$(grep -c -- '--platform=$$BUILDPLATFORM' $$f)" -ge 2 ] || { echo "$$f: build stages must use --platform=\$$BUILDPLATFORM"; ok=0; }; \
	  grep -q 'GOARCH=$$TARGETARCH' $$f || { echo "$$f: go build must cross-compile with GOARCH=\$$TARGETARCH"; ok=0; }; \
	  grep -q 'platforms: linux/amd64,linux/arm64' $$w || { echo "$$w: must build linux/amd64,linux/arm64"; ok=0; }; \
	  [ $$ok = 1 ]

web-install:
	cd web && pnpm install --frozen-lockfile --silent

web-check: web-install ## prettier + eslint + svelte-check + vitest
	cd web && pnpm run --silent lint && pnpm run --silent check && pnpm run --silent test

build-web: web-install ## build the SPA into internal/web/dist (embedded by go build)
	cd web && pnpm run --silent build
	touch internal/web/dist/.gitkeep # the adapter empties dist; go:embed needs the dir in fresh checkouts

build: build-web ## build bin/hashi (with the SPA) for this machine
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/hashi ./cmd/hashi

ENV ?= prod
TAG ?= $(COMMIT)

image: ## build the multi-arch image locally (no push)
	docker buildx build --platform linux/amd64,linux/arm64 -f deploy/docker/Dockerfile \
	  --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t hashi:$(COMMIT) .

deploy: ## deploy TAG (default: short SHA of HEAD) to ENV (deploy/targets/$(ENV).env)
	deploy/deploy.sh deploy $(ENV) $(TAG)

rollback: ## return ENV to the tag that was running before the last deploy
	deploy/deploy.sh rollback $(ENV)

smoke: ## readyz, version, SPA and public listeners on ENV
	deploy/deploy.sh smoke $(ENV) $(if $(filter command line,$(origin TAG)),$(TAG),)

dev: web-install ## Go backend on :8080 with ./data + Vite dev server on :5173 (proxies /api)
	@trap 'kill 0' EXIT; \
	  go run ./cmd/hashi -data-dir ./data -admin-addr 127.0.0.1:8080 -public-apps-addr off -public-status-addr off & \
	  cd web && pnpm run dev

clean:
	rm -rf bin dist web/.svelte-kit
	find internal/web/dist -mindepth 1 ! -name .gitkeep -delete
