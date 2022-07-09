.PHONY: protoc
protoc:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/api.proto

.PHONY: toxfu toxfusaba all
toxfu:
	GOOS=linux GOARCH=amd64 go build -o bin/toxfu ./cmd/toxfu

toxfuarm:
	GOOS=linux GOARCH=arm64 go build -o bin/toxfuarm ./cmd/toxfu

toxfusaba:
	go build -o bin/toxfusaba ./cmd/toxfusaba

all: toxfu toxfusaba

deploy: toxfu toxfuarm toxfusaba
	rsync -avh ./bin/toxfuarm ubuntu@192.168.1.22:/tmp/
	rsync -avh ./bin/toxfu ubuntu@10.28.100.113:/tmp/
