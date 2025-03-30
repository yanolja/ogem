.PHONY: help build build-dev up up-dev down down-dev logs logs-dev exec-app exec-valkey clean clean-all

# Default target when just running 'make'
.DEFAULT_GOAL := help

# Project name for container prefixing
PROJECT_NAME = ogem

# Show help
help:
	@echo "OGEM Docker Commands:"
	@echo "===================="
	@echo "make build         - Build production Docker images"
	@echo "make build-dev     - Build development Docker images"
	@echo "make up            - Start production containers"
	@echo "make up-dev        - Start development containers with hot-reload"
	@echo "make down          - Stop production containers"
	@echo "make down-dev      - Stop development containers"
	@echo "make logs          - View logs from production containers"
	@echo "make logs-dev      - View logs from development containers"
	@echo "make exec-app      - Execute shell in the app container"
	@echo "make exec-valkey   - Execute Redis CLI in the Valkey container"
	@echo "make clean         - Remove containers and networks"
	@echo "make clean-all     - Remove containers, networks, volumes, and images"

# Build production images
build:
	docker compose build

# Build development images
build-dev:
	docker compose -f docker-compose.dev.yml build

# Start production containers
up:
	docker compose up -d

# Start development containers with hot-reload
up-dev:
	docker compose -f docker-compose.dev.yml up -d

# Start and follow logs for development
dev:
	docker compose -f docker-compose.dev.yml up

# Stop production containers
down:
	docker compose down

# Stop development containers
down-dev:
	docker compose -f docker-compose.dev.yml down

# View logs from production containers
logs:
	docker compose logs -f

# View logs from development containers
logs-dev:
	docker compose -f docker-compose.dev.yml logs -f

# Execute shell in the app container
exec-app:
	docker exec -it $(PROJECT_NAME)-app bash || docker exec -it $(PROJECT_NAME)-app sh

# Execute shell in development app container
exec-app-dev:
	docker exec -it $(PROJECT_NAME)-app-dev bash || docker exec -it $(PROJECT_NAME)-app-dev sh

# Execute Redis CLI in the Valkey container
exec-valkey:
	docker exec -it $(PROJECT_NAME)-valkey valkey-cli

# Execute Redis CLI in the development Valkey container
exec-valkey-dev:
	docker exec -it $(PROJECT_NAME)-valkey-dev valkey-cli

# Remove containers and networks
clean:
	docker compose down
	docker compose -f docker-compose.dev.yml down

# Remove containers, networks, volumes, and images
clean-all:
	docker compose down -v --rmi all
	docker compose -f docker-compose.dev.yml down -v --rmi all

# Rebuild and restart development environment
restart-dev: down-dev build-dev up-dev

# List all running containers
ps:
	docker compose ps
	docker compose -f docker-compose.dev.yml ps
