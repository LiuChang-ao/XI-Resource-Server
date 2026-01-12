.PHONY: proto build-cloud build-agent e2e e2e-oss test clean

# Generate protobuf code
proto:
	cd proto && protoc --go_out=. --go_opt=module=github.com/xiresource/proto control.proto

# Build cloud server
build-cloud: proto
	cd cloud && go build -o ../bin/server ./cmd/server

# Build agent
build-agent: proto
	cd agent && go build -o ../bin/agent ./cmd/agent

# Run e2e test (M0)
e2e: proto
	cd scripts && go run M0_e2e.go

# Run e2e test with real OSS
e2e-oss: proto build-cloud build-agent
	cd scripts && go run e2e_oss.go

# Run unit tests
test:
	cd cloud && go test ./...
	cd agent && go test ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf proto/control
