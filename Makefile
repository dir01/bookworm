help: # Show help for each of the Makefile recipes.
	@grep -E '^[a-zA-Z0-9 -]+:.*#'  Makefile | while read -r l; do printf "\033[1;32m$$(echo $$l | cut -f 1 -d':')\033[00m:$$(echo $$l | cut -f 2- -d'#')\n"; done
.PHONY: help

run: # Run the servie (useful for local development)
	DB_PATH=db/sqlite.db go run --tags "fts5" .
.PHONY: run

build: # Build the service binary
	go build --tags "fts5" -o ./bin/bookworm .

test: # Run unit tests
	go test --tags "fts5" ./...
.PHONY: test

docker-test: # Run unit tests in a Docker container
	docker compose run --rm --build bookworm make test
.PHONY: docker-test

precommit: # Run all possible checks before committing
	make vendor
	make tidy
	make generate
	make format
	make build
	make test
.PHONY: precommit

generate: # Generate auto-generated code
	go generate ./...

format: # Format the code
	go fmt ./...

vendor: # Cache dependencies from go.mod into vendor/ directory
	go mod vendor

tidy: # Clean up unused dependencies from go.sum
	go mod tidy

SQL_MIGRATE_CONFIG ?= ./db/dbconfig.yml
SQL_MIGRATE_ENV ?= development

new-migration: # Create a new migration
	sql-migrate new -config "${SQL_MIGRATE_CONFIG}" $(shell bash -c 'read -p "Enter migration name: " name; echo $$name')

migrate: # Migrate the database to the latest version
	go run --tags "fts5" github.com/rubenv/sql-migrate/sql-migrate up -config "${SQL_MIGRATE_CONFIG}" -env "${SQL_MIGRATE_ENV}"
.PHONY: migrate

migrate-down: # Rollback the database one version down
	go run --tags "fts5" github.com/rubenv/sql-migrate/sql-migrate down -config "${SQL_MIGRATE_CONFIG}" -env "${SQL_MIGRATE_ENV}"
.PHONY: migrate-down

