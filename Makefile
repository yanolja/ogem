.PHONY: help build-dev up-dev down-dev logs-dev exec-app-dev exec-valkey-dev clean

# Default target when just running 'make'
.DEFAULT_GOAL := help

# Project name for container prefixing
PROJECT_NAME = ogem

# Show help
help:
	@echo "OGEM Docker Commands:"
	@echo "===================="
	@echo "make build-dev		- Build development Docker images"
	@echo "make up-dev			- Start development containers with hot-reload"
	@echo "make dev				- Start and follow logs for development"
	@echo "make down-dev		- Stop development containers"
	@echo "make logs-dev		- View logs from development containers"
	@echo "make exec-app-dev	- Execute shell in development app container"
	@echo "make exec-valkey-dev	- Execute Redis CLI in development Valkey container"
	@echo "make clean			- Remove containers and networks"
	@echo "make restart-dev		- Rebuild and restart development environment"
	@echo "make ps				- List all running containers"

# Build development images
build-dev:
	docker compose -f docker-compose.dev.yml build

# Start development containers with hot-reload
up-dev:
	docker compose -f docker-compose.dev.yml up -d

# Start and follow logs for development
dev:
	docker compose -f docker-compose.dev.yml up

# Stop development containers
down-dev:
	docker compose -f docker-compose.dev.yml down

# View logs from development containers
logs-dev:
	docker compose -f docker-compose.dev.yml logs -f

# Execute shell in development app container
exec-app-dev:
	docker exec -it $(PROJECT_NAME)-app-dev bash || docker exec -it $(PROJECT_NAME)-app-dev sh

# Execute Redis CLI in the development Valkey container
exec-valkey-dev:
	docker exec -it $(PROJECT_NAME)-valkey-dev valkey-cli

# Remove containers and networks
clean:
	docker compose -f docker-compose.dev.yml down

# Rebuild and restart development environment
restart-dev: down-dev build-dev up-dev

# List all running containers
ps:
	docker compose -f docker-compose.dev.yml ps
