APP_NAME=em
BUILD_DIR=bin
GOFILES=$(shell find . -type f -name '*.go' -not -path "./vendor/*")

# Default values (can be overridden)
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

.PHONY: all build build-all install clean test run mock-jira

# Usage:
#   make build           # builds for your current system, output: bin/em (+ bin/em-<os>-<arch>-<hash>)
#   make build-all       # builds for all supported systems
#   GOOS=linux GOARCH=amd64 make build  # cross-compiles for linux/amd64

all: build

build:
	@echo "Building $(APP_NAME) for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BUILD_DIR)
	GIT_HASH=$$(git rev-parse --short HEAD); \
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "-X 'main.BuildGitHash=$$GIT_HASH' -X 'main.BuildLatestHash=$$GIT_HASH'" -o $(BUILD_DIR)/$(APP_NAME) ./main.go; \

build-all:
	@echo "Building $(APP_NAME) for all supported OS/ARCH combinations..."
	@rm -rf $(BUILD_DIR)
	@mkdir -p $(BUILD_DIR)
	GIT_HASH=$$(git rev-parse --short HEAD); \
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "-X 'main.BuildGitHash=$$GIT_HASH' -X 'main.BuildLatestHash=$$GIT_HASH'" -o $(BUILD_DIR)/$(APP_NAME) ./main.go; \
	GOOS=linux GOARCH=amd64 go build -ldflags "-X 'main.BuildGitHash=$$GIT_HASH' -X 'main.BuildLatestHash=$$GIT_HASH'" -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64-$$GIT_HASH ./main.go; \
	GOOS=linux GOARCH=arm64 go build -ldflags "-X 'main.BuildGitHash=$$GIT_HASH' -X 'main.BuildLatestHash=$$GIT_HASH'" -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64-$$GIT_HASH ./main.go; \
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -ldflags "-X 'main.BuildGitHash=$$GIT_HASH' -X 'main.BuildLatestHash=$$GIT_HASH'" -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64-$$GIT_HASH ./main.go; \
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -ldflags "-X 'main.BuildGitHash=$$GIT_HASH' -X 'main.BuildLatestHash=$$GIT_HASH'" -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64-$$GIT_HASH ./main.go

install: build
	@mkdir -p ~/.local/bin
	@cp $(BUILD_DIR)/$(APP_NAME) ~/.local/bin/$(APP_NAME)
	@echo "Installed $(APP_NAME) to ~/.local/bin/$(APP_NAME)"

clean:
	rm -rf $(BUILD_DIR)

test:
	go test ./...

run:
	go run ./main.go $(ARGS)

mock-jira:
	go run ./internal/testutil/mockjira/cmd $(ARGS)
