.PHONY: build run clean test

build:
	go build -o bin/duet .

run: build
	./bin/duet

clean:
	rm -rf bin/

test:
	go test -v ./...

dev:
	@make -j 2 dev-worker dev-go

dev-worker:
	@echo "Starting Cloudflare Worker..."
	cd cf-worker && bunx wrangler dev --port 8788

dev-go:
	@echo "Starting Duet Go Server..."
	@sleep 3
	go run main.go -worker http://localhost:8788

dev-remote:
	go run main.go -worker https://duet-cf-worker.incident-agent.workers.dev

install:
	go install .
