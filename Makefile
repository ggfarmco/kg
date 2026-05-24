.PHONY: build test gen migrate lint install clean build-extractor build-plugin-treesitter test-all e2e test-scripts

BIN := ./bin/kg
DB  ?= ./kg.db

build:
	@mkdir -p bin
	go build -o $(BIN) ./cmd/kg

test:
	go test ./...

gen:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc generate

migrate:
	go run github.com/pressly/goose/v3/cmd/goose -dir migrations sqlite3 $(DB) up

lint:
	golangci-lint run

install:
	go install ./cmd/kg

clean:
	rm -rf bin *.db *.db-wal *.db-shm

build-extractor: build
	go build -o ./bin/kg-extractor ./cmd/kg-extractor

build-plugin-treesitter:
	CGO_ENABLED=1 go -C ./plugins/tree-sitter build -o ../../bin/kg-extractor-tree-sitter .

test-all: test
	go -C ./plugins/tree-sitter test ./...

e2e: build build-extractor build-plugin-treesitter
	go test -tags=e2e -v ./e2e/...

test-scripts:
	@find .claude-plugin -name '*.test.sh' -print -exec bash {} \;
