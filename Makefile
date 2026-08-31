.PHONY: image up down logs migrate migrate-down migrate-version fmt vet domain-check openapi-check test race integration build check smoke acceptance market-check candle-check candle-quality auth-check ledger-check order-check event-check matching-check derivatives-check backup backup-restore-check web-install web-check web-test web-build web-e2e fuzz-smoke load-test fault-injection security-check release-check

image: web-build
	docker compose build api

up: image
	docker compose up -d --no-build --wait --wait-timeout 120

down:
	docker compose down

logs:
	docker compose logs -f --tail=200

migrate: image
	docker compose run --rm migrate -direction up -timeout 30s

migrate-down: image
	docker compose run --rm migrate -direction down -timeout 30s

migrate-version: image
	docker compose run --rm migrate -direction version -timeout 10s

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './docs-tdd/*')

vet:
	go vet ./...

domain-check:
	go run ./scripts/domaincheck ./internal/trading

openapi-check:
	go run ./scripts/openapicheck ./openapi.yaml

test:
	go test ./...

race:
	go test -race ./...

integration:
	./scripts/integration.sh

build:
	mkdir -p bin
	go build -o bin/api ./cmd/api
	go build -o bin/migrate ./cmd/migrate
	go build -o bin/market-simulator ./cmd/market-simulator
	go build -o bin/market-worker ./cmd/market-worker
	go build -o bin/candle-worker ./cmd/candle-worker
	go build -o bin/event-worker ./cmd/event-worker
	go build -o bin/price-worker ./cmd/price-worker
	go build -o bin/risk-worker ./cmd/risk-worker
	go build -o bin/funding-worker ./cmd/funding-worker
	go build -o bin/mcp-server ./cmd/mcp-server
	go build -o bin/web-gateway ./cmd/web-gateway

check:
	test -z "$$(gofmt -l .)"
	sh -n scripts/*.sh
	if command -v dash >/dev/null 2>&1; then dash -n scripts/*.sh; fi
	go run ./scripts/domaincheck ./internal/trading
	go run ./scripts/openapicheck ./openapi.yaml
	go vet ./...
	! grep -R -n -E '[[:space:]]_ = tx\.Commit\(' internal/platform/postgres
	go test ./...
	go test -race ./...
	mkdir -p bin
	go build -o bin/api ./cmd/api
	go build -o bin/migrate ./cmd/migrate
	go build -o bin/market-simulator ./cmd/market-simulator
	go build -o bin/market-worker ./cmd/market-worker
	go build -o bin/candle-worker ./cmd/candle-worker
	go build -o bin/event-worker ./cmd/event-worker
	go build -o bin/price-worker ./cmd/price-worker
	go build -o bin/risk-worker ./cmd/risk-worker
	go build -o bin/funding-worker ./cmd/funding-worker
	go build -o bin/mcp-server ./cmd/mcp-server
	go build -o bin/web-gateway ./cmd/web-gateway
	$(MAKE) web-install web-check web-test web-build
	./scripts/verify-project.sh

smoke:
	./scripts/smoke.sh

acceptance:
	./scripts/acceptance.sh

market-check:
	go run ./scripts/marketcheck

candle-check:
	go run ./scripts/candlecheck

candle-quality:
	go run ./scripts/candlequality

auth-check:
	./scripts/authcheck.sh

ledger-check:
	./scripts/ledgercheck.sh

order-check:
	./scripts/ordercheck.sh

event-check:
	./scripts/eventcheck.sh

matching-check:
	./scripts/matchingcheck.sh

derivatives-check:
	./scripts/derivatives-check.sh

backup:
	./scripts/backup.sh

backup-restore-check:
	./scripts/backup-restore-check.sh

web-install:
	npm ci --cache output-tdd/npm-cache --prefix web

web-check:
	npm run check --prefix web

web-test:
	npm test --prefix web

web-build:
	npm run build --prefix web

web-e2e:
	npm run test:e2e --prefix web

fuzz-smoke:
	./scripts/fuzz-smoke.sh

load-test:
	./scripts/load-test.sh

fault-injection:
	./scripts/fault-injection.sh

security-check:
	./scripts/security-check.sh

release-check:
	./scripts/release-check.sh
