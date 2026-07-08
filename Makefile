BINARY      := runright
CMD_DIR     := ./cmd/runright
BUILD_FLAGS := -ldflags="-s -w"

PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64

.PHONY: build build-all build-linux build-linux-amd64 build-linux-arm64 \
	test lint clean monitor-test recommend-test catalog-list catalog-update-aws catalog-update-gcp tidy bench-agent

build:
	go build $(BUILD_FLAGS) -o bin/$(BINARY) $(CMD_DIR)

# Build for all platforms — outputs to bin/runright-{os}-{arch}
build-all:
	@for platform in $(PLATFORMS); do \
		os=$$(echo $$platform | cut -d/ -f1); \
		arch=$$(echo $$platform | cut -d/ -f2); \
		output="bin/$(BINARY)-$${os}-$${arch}"; \
		[ "$${os}" = "windows" ] && output="$${output}.exe"; \
		echo "Building $${output}..."; \
		GOOS=$${os} GOARCH=$${arch} go build $(BUILD_FLAGS) -o $${output} $(CMD_DIR); \
	done

# Convenience targets for the most common cases
build-linux:     build-linux-amd64 build-linux-arm64

build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build $(BUILD_FLAGS) -o bin/$(BINARY)-linux-amd64 $(CMD_DIR)

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build $(BUILD_FLAGS) -o bin/$(BINARY)-linux-arm64 $(CMD_DIR)

test:
	go test ./... -v -race

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

monitor-test: build
	./bin/$(BINARY) monitor --duration 30s --interval 2s --export file --output-dir /tmp/runright-test

recommend-test: build
	./bin/$(BINARY) recommend --metrics /tmp/runright-test/metrics-summary.json --format table

catalog-list: build
	./bin/$(BINARY) catalog list

catalog-update-aws: build
	go run ./catalog/updater/aws/... --output catalog/data/aws.json
	cp catalog/data/aws.json internal/catalog/data/aws.json

catalog-update-gcp: build
	go run ./catalog/updater/gcp/... --output catalog/data/gcp.json
	cp catalog/data/gcp.json internal/catalog/data/gcp.json

tidy:
	go mod tidy

bench-agent:
	./scripts/bench-agent.sh
