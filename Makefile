SERVICES := api-gateway identity-service catalog-service order-service payment-service shipping-simulator analytics-service
# Shared modules tested alongside the services.
MODULES := pkg contracts proto/gen/go tests tools/olist-importer

COMPOSE := docker compose -f deployments/docker-compose.yml
BUF := go run github.com/bufbuild/buf/cmd/buf@v1.75.0
# Baseline for buf breaking; CI overrides it with the PR's base branch.
BUF_AGAINST ?= ../.git\#branch=develop,subdir=proto

.PHONY: build test tidy run-% up down proto proto-generate proto-lint proto-breaking check-db-isolation

build:
	@for s in $(SERVICES); do \
		echo "==> build $$s"; \
		(cd services/$$s && go build -o ../../bin/$$s ./cmd/server) || exit 1; \
	done

test:
	@for d in $(addprefix services/,$(SERVICES)) $(MODULES); do \
		echo "==> test $$d"; \
		(cd $$d && go test ./...) || exit 1; \
	done

tidy:
	@for d in $(addprefix services/,$(SERVICES)) $(MODULES); do (cd $$d && GOWORK=off go mod tidy) || exit 1; done

run-%:
	cd services/$* && go run ./cmd/server

up:
	$(COMPOSE) up -d --build --wait

down:
	$(COMPOSE) down

check-db-isolation:
	sh deployments/postgres/check-isolation.sh

proto: proto-generate proto-lint proto-breaking

proto-generate:
	cd proto && $(BUF) generate

proto-lint:
	cd proto && $(BUF) lint

proto-breaking:
	cd proto && $(BUF) breaking --against '$(BUF_AGAINST)'
