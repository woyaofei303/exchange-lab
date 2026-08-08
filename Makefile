.PHONY: up down logs fmt vet test race integration build check smoke acceptance

up:
	docker compose up -d --build --wait --wait-timeout 120

down:
	docker compose down

logs:
	docker compose logs -f --tail=200

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './docs-tdd/*')

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

integration:
	go test -count=1 -race -tags=integration ./internal/health

build:
	mkdir -p bin
	go build -o bin/api ./cmd/api

check:
	test -z "$$(gofmt -l .)"
	sh -n scripts/*.sh
	if command -v dash >/dev/null 2>&1; then dash -n scripts/*.sh; fi
	go vet ./...
	go test ./...
	go test -race ./...
	mkdir -p bin
	go build -o bin/api ./cmd/api
	./scripts/verify-project.sh

smoke:
	./scripts/smoke.sh

acceptance:
	./scripts/acceptance.sh
