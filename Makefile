.PHONY: up down logs sqlc test tidy migrate web up-prod up-obs

COMPOSE := docker compose -f deploy/compose/docker-compose.yml --env-file deploy/compose/.env
COMPOSE_PROD := docker compose -f deploy/compose/docker-compose.yml -f deploy/compose/docker-compose.prod.yml --env-file deploy/compose/.env

up:
	@test -f deploy/compose/.env || (echo "copy deploy/compose/.env.example to deploy/compose/.env" && exit 1)
	$(COMPOSE) up -d --build

up-prod:
	@test -f deploy/compose/.env || (echo "copy deploy/compose/.env.example to deploy/compose/.env" && exit 1)
	$(COMPOSE_PROD) up -d --build

up-obs:
	@test -f deploy/compose/.env || (echo "copy deploy/compose/.env.example to deploy/compose/.env" && exit 1)
	$(COMPOSE) --profile obs up -d --build

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f api worker migrate

sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate

test:
	go test ./...

web:
	cd web && npm ci && npm run build

tidy:
	go mod tidy

migrate:
	go run ./cmd/sms migrate
