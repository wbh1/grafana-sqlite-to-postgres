# The binary to build (just the basename).
BIN := grafana-migrate
SRC_DIR := ./cmd/grafana-migrate

# This version-strategy uses git tags to set the version string
VERSION := $(shell git describe --tags --always --dirty)
OS := $(shell go env GOOS)
ARCH := $(shell go env GOARCH)


build:
	go build -o dist/$(BIN)_$(OS)_$(ARCH)-$(VERSION) $(SRC_DIR)

build-all: 
	env GOOS=darwin GOARCH=amd64 go build -o dist/$(BIN)_darwin_amd64-$(VERSION) $(SRC_DIR)
	env GOOS=linux GOARCH=amd64 go build -o dist/$(BIN)_linux_amd64-$(VERSION) $(SRC_DIR)
	env GOOS=windows GOARCH=amd64 go build -o dist/$(BIN)_windows_amd64-$(VERSION) $(SRC_DIR)

clean:
	rm dist/*