.PHONY: up down logs fmt vet test race build check smoke acceptance

up:
	docker compose up -d --build

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

build:
	go build ./cmd/api

check:
	test -z "$$(gofmt -l .)"
	go vet ./...
	go test ./...
	go test -race ./...
	go build ./cmd/api
	docker compose config >/dev/null

smoke:
	./scripts/smoke.sh

acceptance:
	./scripts/acceptance.sh
