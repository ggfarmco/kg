.PHONY: build test gen migrate lint install clean

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
