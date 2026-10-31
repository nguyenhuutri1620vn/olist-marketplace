SERVICES := api-gateway identity-service catalog-service order-service payment-service shipping-simulator analytics-service
# Shared modules tested alongside the services.
MODULES := pkg contracts proto/gen/go tests tools/olist-importer

COMPOSE := docker compose -f deployments/docker-compose.yml
BUF := go run github.com/bufbuild/buf/cmd/buf@v1.75.0
# Kaggle CSVs that `make import` copies into the importer container.
OLIST_FILES := product_category_name_translation.csv olist_sellers_dataset.csv olist_products_dataset.csv olist_orders_dataset.csv olist_order_items_dataset.csv olist_customers_dataset.csv
IMPORTER := $(COMPOSE) --profile tools
# Baseline for buf breaking; CI overrides it with the PR's base branch.
BUF_AGAINST ?= ../.git\#branch=develop,subdir=proto

.PHONY: build test tidy run-% up down import web-test proto proto-generate proto-lint proto-breaking check-db-isolation

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

# The web app's lint, tests and build (npm 11 / Node 24; see web/README.md).
web-test:
	cd web && npm ci && npm run lint && npm test && npm run build

up:
	$(COMPOSE) up -d --build --wait

down:
	$(COMPOSE) down

# Load Categories, Sellers and Products into catalog and seed the 41 demo
# Accounts into identity (idempotent; FR15, FR17). DEMO_PASSWORD overrides the
# demo password.
# tar --no-xattrs: macOS tags downloads with xattrs that docker cp cannot set.
import:
	@for f in $(OLIST_FILES); do test -f data/$$f || { echo "missing data/$$f: download the Olist dataset from Kaggle into data/"; exit 1; }; done
	$(COMPOSE) up -d --wait catalog-service identity-service
	$(IMPORTER) rm -fsv olist-importer
	$(IMPORTER) create --build olist-importer
	COPYFILE_DISABLE=1 tar --no-xattrs -C data -cf - $(OLIST_FILES) | docker cp - "$$($(IMPORTER) ps -aq olist-importer)":/data
	docker start -a "$$($(IMPORTER) ps -aq olist-importer)"

check-db-isolation:
	sh deployments/postgres/check-isolation.sh

proto: proto-generate proto-lint proto-breaking

proto-generate:
	cd proto && $(BUF) generate

proto-lint:
	cd proto && $(BUF) lint

proto-breaking:
	cd proto && $(BUF) breaking --against '$(BUF_AGAINST)'
