module github.com/xiresource/agent

go 1.21

require (
	github.com/gorilla/websocket v1.5.1
	github.com/xiresource/proto/control v0.0.0
	google.golang.org/protobuf v1.31.0
)

replace github.com/xiresource/proto => ../proto
