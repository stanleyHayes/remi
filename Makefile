.DEFAULT_GOAL := help
.PHONY: help deps up down api web admin seed seed-reset test lint check

help: ## Show available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

deps: ## Install Go and JS dependencies
	cd apps/api && go mod tidy
	pnpm install

up: ## Start MongoDB (localhost:27019)
	docker compose up -d mongo

down: ## Stop local infrastructure
	docker compose down

api: ## Run the Go API on :8088
	cd apps/api && go run ./cmd/server

web: ## Run the public site on :3010
	pnpm --filter web dev

admin: ## Run the admin app on :3011
	pnpm --filter admin dev

seed: ## Seed the development database (idempotent upsert)
	cd apps/api && go run ./cmd/seed

seed-reset: ## Drop the seeded collections, then seed again
	cd apps/api && go run ./cmd/seed -reset

test: ## Run Go tests (integration tests skip without MongoDB)
	cd apps/api && go test ./...

lint: ## Run go vet on the API
	cd apps/api && go vet ./...

check: ## Everything CI runs: vet, build, tests, frontend builds
	cd apps/api && go vet ./...
	cd apps/api && go build ./...
	cd apps/api && go test ./...
	pnpm --filter web build
	pnpm --filter admin build
