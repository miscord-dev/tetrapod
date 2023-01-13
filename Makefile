# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.25.0
ENVTEST ?= $(LOCALBIN)/setup-envtest

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

deploy: toxfu toxfuarm toxfusaba
	rsync -avh ./bin/toxfuarm ubuntu@192.168.1.22:/tmp/
	rsync -avh ./bin/toxfu ubuntu@10.28.100.113:/tmp/

arena:
	rsync -avh ./toxfud/bin/manager ubuntu@160.248.79.94:/home/ubuntu/

.PHONY: init
init:
	aqua cp -o bin cnitool bandwidth bridge dhcp firewall host-device host-local ipvlan loopback macvlan portmap ptp sbr static tuning vlan vrf

.PHONY: toxfu-extra-routes toxfu-pod-ipam hostvrf cni-plugins
toxfu-extra-routes:
	go build -o ./bin ./toxfucni/cmd/toxfu-extra-routes

toxfu-pod-ipam:
	go build -o ./bin ./toxfucni/cmd/toxfu-pod-ipam

hostvrf:
	go build -o ./bin ./toxfucni/cmd/hostvrf

cni-plugins: toxfu-pod-ipam hostvrf

.PHONY: test
test: envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test -p 1 -exec "sudo -E" ./... -coverprofile cover.out

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
