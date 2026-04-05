.PHONY: up down seed test lint swagger cover migrate build

up:
	docker compose up --build -d
	@echo "Waiting for services to be ready..."
	@sleep 8
	@echo "Services are up at http://localhost:8080"
	@$(MAKE) seed

down:
	docker compose down -v

seed:
	@chmod +x scripts/seed.sh
	@bash scripts/seed.sh http://localhost:8080

build:
	@for svc in auth-service room-service availability-service booking-service conference-mock-service api-gateway; do \
		echo "Building $$svc..."; \
		cd services/$$svc && go build ./cmd/ && cd ../..; \
	done

test:
	@echo "Running unit tests..."
	@for svc in auth-service room-service availability-service booking-service; do \
		echo "Testing $$svc..."; \
		cd services/$$svc && go test ./... -v -count=1 && cd ../..; \
	done
	@cd pkg && go test ./... -v -count=1 && cd ..

cover:
	@echo "Running tests with coverage..."
	@mkdir -p coverage
	@for svc in auth-service room-service availability-service booking-service; do \
		echo "Coverage $$svc..."; \
		cd services/$$svc && go test ./... -coverprofile=../../coverage/$$svc.out -count=1 && cd ../..; \
	done
	@cd pkg && go test ./... -coverprofile=../coverage/pkg.out -count=1 && cd ..
	@echo "Coverage reports in coverage/"

test-e2e:
	@echo "Running E2E tests..."
	cd tests/e2e && go test -v -count=1 -timeout 120s ./...

lint:
	@golangci-lint run ./...

swagger:
	@echo "Swagger is served from api.yaml at http://localhost:8080"
	@echo "Copy api.yaml to services/api-gateway if needed"

migrate:
	docker compose run --rm migrate

logs:
	docker compose logs -f

restart:
	docker compose restart
