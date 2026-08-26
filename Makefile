BINARY=grok-webui
ADDR=:8080
DATA=./data

# git tag/commit stamp (falls back to "dev" outside a repo)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

LDFLAGS=-X main.version=$(VERSION)

.PHONY: build test run clean tidy vet docker-image docker-up docker-down docker-logs install

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/server

run: build
	./$(BINARY) --addr $(ADDR) --data $(DATA)

tidy:
	go mod tidy

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY)
	rm -rf $(DATA)/*.db*

# ---- 24/7 / service management -------------------------------------------

docker-image:
	docker build -t grok-webui:local .

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f webui

# interactive installer TUI (native systemd service or docker compose)
install:
	./scripts/install.sh
