# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.25.0
ENVTEST ?= $(LOCALBIN)/setup-envtest

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

deploy: tetrapod tetrapodarm tetrapodsaba
	rsync -avh ./bin/tetrapodarm ubuntu@192.168.1.22:/tmp/
	rsync -avh ./bin/tetrapod ubuntu@10.28.100.113:/tmp/

arena:
	rsync -avh ./tetrad/bin/manager ubuntu@160.248.79.94:/home/ubuntu/

.PHONY: init
init:
	aqua cp -o bin cnitool bandwidth bridge dhcp firewall host-device host-local ipvlan loopback macvlan portmap ptp sbr static tuning vlan vrf

.PHONY: tetra-extra-routes tetra-pod-ipam hostvrf cni-plugins

bin:
	mkdir -p bin

tetra-extra-routes: bin
	CGO_ENABLED=0 go build -o ./bin ./tetracni/cmd/tetra-extra-routes

tetra-pod-ipam: bin
	CGO_ENABLED=0 go build -o ./bin ./tetracni/cmd/tetra-pod-ipam

hostvrf: bin
	CGO_ENABLED=0 go build -o ./bin ./tetracni/cmd/hostvrf

route-pods: bin
	CGO_ENABLED=0 go build -o ./bin ./tetracni/cmd/route-pods

nsexec: bin
	CGO_ENABLED=0 go build -o ./bin ./tetracni/cmd/nsexec

cni-plugins: tetra-extra-routes tetra-pod-ipam hostvrf route-pods nsexec

.PHONY: test
test: envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test -p 1 -exec "sudo -E" ./... -coverprofile cover.out

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: install-cnitools
install-cnitools:
	GOBIN=$(PWD)/bin go install github.com/containernetworking/cni/cnitool@latest
	( \
		rm -rf plugins; \
		git clone https://github.com/containernetworking/plugins.git && \
		cd plugins && \
		./build_linux.sh && \
		mv ./bin/* ../bin && \
		cd .. && \
		rm -rf plugins \
	)
