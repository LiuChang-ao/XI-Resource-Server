module github.com/xiresource/cloud

go 1.21.0

require (
	github.com/gorilla/websocket v1.5.1
	github.com/joho/godotenv v1.5.1
	github.com/redis/go-redis/v9 v9.17.2
	github.com/tencentyun/cos-go-sdk-v5 v0.7.72
	github.com/xiresource/proto/control v0.0.0
	google.golang.org/protobuf v1.31.0
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/clbanning/mxj v1.8.4 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/go-sql-driver/mysql v1.9.3 // indirect
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-sqlite3 v1.14.33 // indirect
	github.com/mitchellh/mapstructure v1.4.3 // indirect
	github.com/mozillazg/go-httpheader v0.2.1 // indirect
	golang.org/x/net v0.17.0 // indirect
)

replace github.com/xiresource/proto => ../proto
