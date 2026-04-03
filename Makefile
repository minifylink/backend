EXCLUDE := mocks|repository

test:
	@echo "running unit tests…"
	@go test -cover -count=1 \
		$$(go list ./internal/... ./cmd/... | grep -Ev '/$(EXCLUDE)')

test-coverage:
	@echo "running unit tests with coverage…"
	@go test -coverprofile=coverage.out -count=1 \
		$$(go list ./internal/... | grep -Ev '/$(EXCLUDE)')

coverage:
	@go tool cover -func=coverage.out

coverage-html:
	@go tool cover -html=coverage.out -o coverage.html

test-integration:
	@echo "running integration tests…"
	@go test -tags=integration -v -count=1 ./internal/repository/...

test-e2e:
	@echo "running e2e tests…"
	@go test -tags=e2e -v -count=1 ./tests/e2e/...
