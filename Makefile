SERVICES := api-gateway catalog-service customer-service order-service payment-service review-service analytics-service

.PHONY: build test tidy run-% up down proto

build:
	@for s in $(SERVICES); do \
		echo "==> build $$s"; \
		(cd services/$$s && go build -o ../../bin/$$s ./cmd/server) || exit 1; \
	done

test:
	@for s in $(SERVICES); do (cd services/$$s && go test ./...) || exit 1; done

tidy:
	@for s in $(SERVICES); do (cd services/$$s && go mod tidy); done

run-%:
	cd services/$* && go run ./cmd/server

up:
	docker compose -f deployments/docker-compose.yml up -d --build

down:
	docker compose -f deployments/docker-compose.yml down

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/*/v1/*.proto
