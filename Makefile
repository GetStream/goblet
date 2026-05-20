.PHONY: all build test vet lint tidy fmt modernize docker clean

GO      ?= go
PKGS    := ./...
BIN_DIR := bin

all: build

build:
	$(GO) build -o $(BIN_DIR)/goblet-server ./goblet-server
	$(GO) build -o $(BIN_DIR)/packobjectshook ./hooks/packobjects

test:
	$(GO) test $(PKGS)

vet:
	$(GO) vet $(PKGS)

lint: vet
	golangci-lint run

tidy:
	$(GO) mod tidy

fmt:
	gofmt -w .

modernize:
	$(GO) run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -fix $(PKGS)

docker:
	docker build -t goblet:dev .

clean:
	rm -rf $(BIN_DIR)
