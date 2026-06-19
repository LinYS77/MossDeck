.PHONY: build run test vet fmt tidy clean web-install web-dev web-build web-preview wallpapers

BINARY := bin/server

## build: compile the server binary into ./bin
build:
	mkdir -p bin
	go build -o $(BINARY) ./cmd/server

## run: build then run locally (reads APP_* from the environment / .env)
run: build
	./$(BINARY)

## test: run the full test suite
test:
	go test ./...

## vet: run go vet
vet:
	go vet ./...

## fmt: format all Go sources
fmt:
	gofmt -s -w .

## tidy: sync dependencies
tidy:
	go mod tidy

## clean: remove build artifacts
clean:
	rm -rf bin

## web-install: install frontend dependencies (pnpm)
web-install:
	cd web && pnpm install

## web-dev: run the Vite dev server (proxies /api -> :8080)
web-dev:
	cd web && pnpm dev

## web-build: typecheck + production build the frontend
web-build:
	cd web && pnpm build

## web-preview: serve the built frontend
web-preview:
	cd web && pnpm preview

## wallpapers: regenerate web-friendly wallpaper copies into web/public/wallpaper
wallpapers:
	python3 scripts/gen-wallpapers.py
