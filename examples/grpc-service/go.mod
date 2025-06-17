module github.com/example/calculator

go 1.24

require (
	connectrpc.com/connect v1.18.1
	connectrpc.com/grpcreflect v1.2.0
	golang.org/x/net v0.35.0
	google.golang.org/protobuf v1.36.5
)

replace github.com/example/calculator => ./
