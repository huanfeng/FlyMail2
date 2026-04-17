.PHONY: test test-integration test-e2e test-all test-e2e-setup test-e2e-teardown

# Unit tests (no external dependencies)
test:
	go test ./internal/core/... ./internal/config/... ./internal/dispatcher/... -v -count=1

# Integration tests (needs SQLite, uses httptest)
test-integration:
	go test ./internal/api/... -v -count=1

# E2E setup: start Mailpit
test-e2e-setup:
	docker compose -f docker-compose.test.yml up -d
	@echo "Waiting for Mailpit to be ready..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		curl -s http://localhost:8025/api/v1/messages > /dev/null 2>&1 && break; \
		echo "  Attempt $$i: Mailpit not ready yet..."; \
		sleep 1; \
	done
	@echo "Mailpit is ready."

# E2E teardown: stop Mailpit
test-e2e-teardown:
	docker compose -f docker-compose.test.yml down

# E2E tests (needs Mailpit running)
test-e2e: test-e2e-setup
	E2E_MAILPIT=1 go test ./internal/e2e/... -v -count=1 -timeout 60s || ($(MAKE) test-e2e-teardown && exit 1)
	$(MAKE) test-e2e-teardown

# Run all tests
test-all: test test-integration test-e2e

# Build
build:
	go build -o mail2im ./cmd/server/main.go
