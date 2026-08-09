.PHONY: run migrate build test ent-gen tidy

BIN_MAIN := 6url-api
BIN_ADMIN := 6url-admin

run:
	go run ./cmd/6url

migrate:
	go run ./cmd/6url-admin migrate

build:
	go build -o $(BIN_MAIN) ./cmd/6url
	go build -o $(BIN_ADMIN) ./cmd/6url-admin

test:
	go test ./...

ent-gen:
	go generate ./ent

tidy:
	go mod tidy
