# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.25.0
ENVTEST ?= $(LOCALBIN)/setup-envtest

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

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

arena:
	go build ./cmd/toxfutest/
	rsync -avh ./toxfutest ubuntu@160.248.79.94:/home/ubuntu/

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./... -coverprofile cover.out

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
