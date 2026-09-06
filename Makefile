GO_MODULES := pkg \
	services/api-gateway \
	services/auth-service \
	services/room-service \
	services/availability-service \
	services/booking-service \
	services/conference-mock-service
SELECTED_GO := $(shell go env GOROOT)/bin/go
GATEWAY_PORT ?= 8080
BASE_URL ?= http://localhost:$(GATEWAY_PORT)

.PHONY: up down seed build test test-race test-e2e cover fmt-check vet verify vuln lint swagger migrate logs restart compose-config

up:
	GATEWAY_PORT=$(GATEWAY_PORT) TEST_TASK_MODE=true docker compose up --build -d --wait --wait-timeout 180
	@$(MAKE) seed GATEWAY_PORT=$(GATEWAY_PORT)
	@echo "Services are ready at http://localhost:$(GATEWAY_PORT)"

down:
	docker compose down -v

seed:
	@bash scripts/seed.sh http://localhost:$(GATEWAY_PORT)

build:
	set -e; for module_dir in $(GO_MODULES); do \
		echo "Building $$module_dir"; \
		(cd $$module_dir && go build ./...); \
	done

test:
	@set -e; for module_dir in $(GO_MODULES); do \
		echo "Testing $$module_dir"; \
		(cd $$module_dir && go test ./... -count=1); \
	done

test-race:
	@set -e; for module_dir in $(GO_MODULES); do \
		echo "Race testing $$module_dir"; \
		(cd $$module_dir && go test ./... -race -count=1); \
	done

test-e2e:
	@cd tests/e2e && BASE_URL=$(BASE_URL) go test -v -count=1 -timeout 120s ./...

cover:
	@mkdir -p coverage
	@coverage_dir="$(CURDIR)/coverage"; \
	set -e; for module_dir in $(GO_MODULES); do \
		module_name=$$(basename $$module_dir); \
		echo "Coverage $$module_dir"; \
		(cd $$module_dir && go test ./... -coverprofile="$$coverage_dir/$$module_name.out" -count=1); \
	done
	@echo "Coverage reports are in coverage/"

fmt-check:
	@unformatted=$$(gofmt -l $$(find pkg services tests -type f -name '*.go')); \
	if [ -n "$$unformatted" ]; then \
		echo "The following files need gofmt:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet:
	@set -e; for module_dir in $(GO_MODULES); do \
		echo "Vetting $$module_dir"; \
		(cd $$module_dir && go vet ./...); \
	done

compose-config:
	docker compose config --quiet

verify: fmt-check vet test-race build compose-config

vuln:
	@set -e; for module_dir in $(GO_MODULES); do \
		echo "Scanning $$module_dir"; \
		(cd $$module_dir && GOTOOLCHAIN=local "$(SELECTED_GO)" run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...); \
	done

lint:
	@set -e; for module_dir in $(GO_MODULES); do \
		echo "Linting $$module_dir"; \
		(cd $$module_dir && golangci-lint run ./...); \
	done

swagger:
	@echo "OpenAPI specification: api.yaml"

migrate:
	docker compose run --rm migrate

logs:
	docker compose logs -f

restart:
	docker compose restart
