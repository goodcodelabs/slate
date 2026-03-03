BINARY := slate-server
BUILD_DIR := bin

.PHONY: all build run test integrate clean

all: build

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/server

run: build
	./$(BUILD_DIR)/$(BINARY)

test:
	go test ./...

integrate: build
	go run ./cmd/integration

clean:
	rm -rf $(BUILD_DIR)
