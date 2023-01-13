LINUX_VERSION=5.18.14

linux-$(LINUX_VERSION).tar.xz:
	wget https://cdn.kernel.org/pub/linux/kernel/v5.x/linux-$(LINUX_VERSION).tar.xz



.PHONY: init
init: linux-$(LINUX_VERSION).tar.xz
	tar xf linux-$(LINUX_VERSION).tar.xz
	cd linux-$(LINUX_VERSION)/tools/lib/bpf && \
		make bpf_helper_defs.h

	git clone https://github.com/xdp-project/xdp-tools.git
