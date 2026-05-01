.PHONY: dev test build lint migrate migration fmt vet clean help

SHELL := /bin/bash
ROOT_DIR := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
MIGRATION_DIR := packages/tpt-pay-core/migrations

## help: show this help message
help:
	@echo "TPT Platform — available targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/  /'

## dev: start full local development stack via Docker Compose
dev:
	docker compose up --build

## dev-down: stop local development stack
dev-down:
	docker compose down -v

## test: run all tests across all Go packages
test:
	go test -count=1 -race ./packages/...

## test-pay: run tpt-pay-core tests only
test-pay:
	go test -count=1 -race ./packages/tpt-pay-core/...

## test-integration: run integration tests (requires running Docker stack)
test-integration:
	docker compose -f docker-compose.test.yml up --build --abort-on-container-exit --exit-code-from test-runner

## build: build all service binaries
build:
	go build -o bin/tpt-pay-server ./packages/tpt-pay-core/cmd/server

## lint: run all linters
lint:
	golangci-lint run ./packages/...

## fmt: format all Go code
fmt:
	gofmt -w ./packages/

## vet: run go vet
vet:
	go vet ./packages/...

## migrate: apply pending database migrations
migrate:
	atlas migrate apply \
		--dir "file://$(MIGRATION_DIR)" \
		--url "$(DATABASE_URL)"

## migrate-status: show migration status
migrate-status:
	atlas migrate status \
		--dir "file://$(MIGRATION_DIR)" \
		--url "$(DATABASE_URL)"

## migration: create a new migration file (usage: make migration name=add_foo)
migration:
ifndef name
	$(error name is required: make migration name=your_migration_name)
endif
	@timestamp=$$(date -u +%Y%m%d%H%M%S); \
	filename="$(MIGRATION_DIR)/$${timestamp}_$(name).sql"; \
	echo "-- Migration: $(name)" > $$filename; \
	echo "-- Created: $$(date -u)" >> $$filename; \
	echo "" >> $$filename; \
	echo "-- Write your migration SQL here." >> $$filename; \
	echo "" >> $$filename; \
	echo "Created: $$filename"

## clean: remove built binaries
clean:
	rm -rf bin/
