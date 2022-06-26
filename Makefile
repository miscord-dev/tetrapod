.PHONY: protoc
protoc:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/api.proto

.PHONY: toxfu toxfusaba all
toxfu:
	go build -o bin/toxfu ./cmd/toxfu

toxfusaba:
	go build -o bin/toxfusaba ./cmd/toxfusaba

all: toxfu toxfusaba
