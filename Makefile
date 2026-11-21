.PHONY: dev infra up down logs swagger build test lint seed fresh reset-db reset-redis reset-ratelimit reset-locks

infra:        ## start postgres + redis only
	docker compose up -d postgres redis

dev: infra    ## run API locally with hot reload
	air

up:           ## full stack in docker
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f api

swagger:      ## regenerate docs/ (swagger.json, swagger.yaml, docs.go)
	swag init -g cmd/server/main.go -o docs --md docs/markdown --parseInternal

VERSION ?= 1.0.0
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -s -w -X cholobangladesh-server/pkg/buildinfo.Version=$(VERSION) -X cholobangladesh-server/pkg/buildinfo.Commit=$(COMMIT)

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/server ./cmd/server

test:
	go test ./...

lint:
	go vet ./...

# ---- data / state (dev). Inside Docker use: docker compose exec api manage <cmd>
seed:            ## 3 tourists, 3 guides, 1 admin (password: Password123)
	go run ./cmd/manage seed

fresh:           ## drop DB + flush Redis + seed
	go run ./cmd/manage fresh
	docker compose restart api

reset-db:
	go run ./cmd/manage reset-db
	docker compose restart api

reset-redis:
	go run ./cmd/manage reset-redis

reset-ratelimit:
	go run ./cmd/manage reset-ratelimit

reset-locks:
	go run ./cmd/manage reset-locks
