.PHONY: help dev server agent dashboard build agents test lint tidy

help:
	@echo "Mission Control"
	@echo "  make server      run the Go server"
	@echo "  make agent       run the Go agent"
	@echo "  make dashboard   run the dashboard dev server"
	@echo "  make build       build server + agent binaries"
	@echo "  make agents      cross-build agent binaries for all platforms (served at /download)"
	@echo "  make test        run Go + JS tests"
	@echo "  make lint        run go vet + eslint"
	@echo "  make tidy        go mod tidy + pnpm install"

server:
	MC_API_KEY=dev-key MC_AGENT_BIN_DIR=bin/agents go run ./apps/server --config examples/server.yaml

agent:
	go run ./apps/agent --config examples/agent.yaml

dashboard:
	pnpm --filter @mc/dashboard dev

# Build the dashboard SPA into apps/dashboard/dist for single-server hosting.
dashboard-build:
	pnpm --filter @mc/dashboard build

# Production single-server: build SPA + cross-build agents, then run ONE server
# that hosts the dashboard (SPA), the API (/api/v1), the WebSocket, and agent
# binary downloads. Modern-SaaS style: backend serves everything.
serve-prod: dashboard-build agents
	MC_STATIC_DIR=apps/dashboard/dist MC_AGENT_BIN_DIR=bin/agents \
		go run ./apps/server --config examples/server.yaml

build:
	go build -o bin/mission-control-server ./apps/server
	go build -o bin/mission-control-agent ./apps/agent

# Print a fresh Web Push VAPID key pair (for blocked-session notifications).
vapid:
	@go run ./cmd/vapidgen

# Cross-build the agent for every supported platform into bin/agents/. The server
# serves these at /download/<name> for the install scripts. Names MUST match what
# install.sh / install.ps1 request: mission-control-agent-<os>-<arch>[.exe].
agents:
	@mkdir -p bin/agents
	@echo "Cross-building agent binaries -> bin/agents/"
	GOOS=darwin  GOARCH=arm64 go build -o bin/agents/mission-control-agent-darwin-arm64      ./apps/agent
	GOOS=darwin  GOARCH=amd64 go build -o bin/agents/mission-control-agent-darwin-amd64      ./apps/agent
	GOOS=linux   GOARCH=amd64 go build -o bin/agents/mission-control-agent-linux-amd64       ./apps/agent
	GOOS=linux   GOARCH=arm64 go build -o bin/agents/mission-control-agent-linux-arm64       ./apps/agent
	GOOS=windows GOARCH=amd64 go build -o bin/agents/mission-control-agent-windows-amd64.exe ./apps/agent
	GOOS=windows GOARCH=arm64 go build -o bin/agents/mission-control-agent-windows-arm64.exe ./apps/agent
	@echo "Done:" && ls -1 bin/agents/

test:
	go test ./...
	pnpm -r test

lint:
	go vet ./...
	pnpm -r typecheck

tidy:
	go mod tidy
	pnpm install
