APP_NAME=devctl-em
BUILD_DIR=bin
GOFILES=$(shell find . -type f -name '*.go' -not -path "./vendor/*")

# Default values (can be overridden)
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

.PHONY: all build build-all clean test

# Usage:
#   make build           # builds for your current system, output: bin/devctl-em-<os>-<arch>-<hash>
#   make build-all       # builds for all supported systems
#   GOOS=linux GOARCH=amd64 make build  # cross-compiles for linux/amd64

all: build

build:
	@echo "Building $(APP_NAME) for $(GOOS)/$(GOARCH)..."
	@rm -rf $(BUILD_DIR)
	@mkdir -p $(BUILD_DIR)
	GIT_HASH=$$(git rev-parse --short HEAD); \
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "-X 'main.BuildGitHash=$$GIT_HASH' -X 'main.BuildLatestHash=$$GIT_HASH'" -o $(BUILD_DIR)/$(APP_NAME)-$(GOOS)-$(GOARCH)-$$GIT_HASH ./main.go

build-all:
	@echo "Building $(APP_NAME) for all supported OS/ARCH combinations..."
	@rm -rf $(BUILD_DIR)
	@mkdir -p $(BUILD_DIR)
	GIT_HASH=$$(git rev-parse --short HEAD); \
	GOOS=linux GOARCH=amd64 go build -ldflags "-X 'main.BuildGitHash=$$GIT_HASH' -X 'main.BuildLatestHash=$$GIT_HASH'" -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64-$$GIT_HASH ./main.go; \
	GOOS=linux GOARCH=arm64 go build -ldflags "-X 'main.BuildGitHash=$$GIT_HASH' -X 'main.BuildLatestHash=$$GIT_HASH'" -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64-$$GIT_HASH ./main.go; \
	GOOS=darwin GOARCH=amd64 go build -ldflags "-X 'main.BuildGitHash=$$GIT_HASH' -X 'main.BuildLatestHash=$$GIT_HASH'" -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64-$$GIT_HASH ./main.go; \
	GOOS=darwin GOARCH=arm64 go build -ldflags "-X 'main.BuildGitHash=$$GIT_HASH' -X 'main.BuildLatestHash=$$GIT_HASH'" -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64-$$GIT_HASH ./main.go

clean:
	rm -rf $(BUILD_DIR)

test:
	go test ./...
