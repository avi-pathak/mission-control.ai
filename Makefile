.PHONY: help dev server agent dashboard build test lint tidy

help:
	@echo "Mission Control"
	@echo "  make server      run the Go server"
	@echo "  make agent       run the Go agent"
	@echo "  make dashboard   run the dashboard dev server"
	@echo "  make build       build server + agent binaries"
	@echo "  make test        run Go + JS tests"
	@echo "  make lint        run go vet + eslint"
	@echo "  make tidy        go mod tidy + pnpm install"

server:
	MC_API_KEY=dev-key go run ./apps/server --config examples/server.yaml

agent:
	go run ./apps/agent --config examples/agent.yaml

dashboard:
	pnpm --filter @mc/dashboard dev

build:
	go build -o bin/mission-control-server ./apps/server
	go build -o bin/mission-control-agent ./apps/agent

test:
	go test ./...
	pnpm -r test

lint:
	go vet ./...
	pnpm -r typecheck

tidy:
	go mod tidy
	pnpm install
