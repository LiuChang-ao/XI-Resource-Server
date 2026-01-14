module github.com/xiresource/agent

go 1.21

require (
	github.com/gorilla/websocket v1.5.1
	github.com/xiresource/proto v0.0.0
	google.golang.org/protobuf v1.31.0
)

require golang.org/x/net v0.17.0 // indirect

replace github.com/xiresource/proto => ../proto
