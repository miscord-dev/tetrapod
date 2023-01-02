// +build ignore

#include <stddef.h>
#include <linux/bpf.h>
#include <linux/in.h>
#include <linux/if_ether.h>
#include <linux/if_packet.h>
#include <linux/ipv6.h>
#include <stdbool.h>
#include <linux/icmpv6.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>
#include <xdp/parsing_helpers.h>

struct event {
	__u32 len;
	__u8  packet[2048];
};

struct meta_info {
	__u32 mark;
} __attribute__((aligned(4)));

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 24);
} events SEC(".maps");

// Force emitting struct event into the ELF.
const struct event *unused __attribute__((unused));

static inline
bool is_disco_packet(unsigned char* payload, unsigned* data_end) {
	if (payload + 1 > data_end) {
		return false;
	}
	
	if (payload[0] & 128) {
		return true;
	}

	return false;
}

static inline
bool is_stun_packet(unsigned char* payload, unsigned* data_end) {
	if (payload + 8 > data_end) {
		return false;
	}

	__u32* magic_cookie = (__u32*)(payload + 4);

	if (*magic_cookie == bpf_htonl(0x2112A442)) {
		return true;
	}

	return false;
}

volatile const unsigned short port = 63455;

SEC("xdp_packet_parser")
int  xdp_parser_func(struct xdp_md *ctx)
{
	void *data_end = (void *)(long)ctx->data_end;
	void *data = (void *)(long)ctx->data;
	struct ethhdr *eth;
	struct iphdr *v4hdr;
	struct ipv6hdr *v6hdr;

	struct hdr_cursor nh;
	int ret;

	nh.pos = data;

	ret = parse_ethhdr(&nh, data_end, &eth);
	const void* pkt_after_ethframe = nh.pos;
	switch (ret) {
	case bpf_htons(ETH_P_IP):
		if ((ret = parse_iphdr(&nh, data_end, &v4hdr)) == -1) {
			return XDP_PASS;
		}
		
		if (ret != IPPROTO_UDP) {
			return XDP_PASS;
		}

		break;
	case bpf_htons(ETH_P_IPV6):
		if ((ret = parse_ip6hdr(&nh, data_end, &v6hdr)) == -1) {
			return XDP_PASS;
		}
		
		if (ret != IPPROTO_UDP) {
			return XDP_PASS;
		}

		break;
	default:
		return XDP_PASS;
	}

	struct udphdr *udphdr;

	int length = parse_udphdr(&nh, data_end, &udphdr);
	if (length == -1) {
		return XDP_PASS;
	}

	if (udphdr->dest != bpf_htons(port)) {
		return XDP_PASS;
	}

	__u8* payload = nh.pos;

	if (!is_disco_packet(payload, data_end) && !is_stun_packet(payload, data_end)) {
		return XDP_PASS;
	}

	struct event *buf_packet;

	buf_packet = bpf_ringbuf_reserve(&events, sizeof(struct event), 0);
	if (!buf_packet) {
		return XDP_PASS;
	}

	__u32 len = data_end - data;
	buf_packet->len = len;

	const long err = bpf_probe_read_kernel(buf_packet->packet, len & 2047, data);
	if (err != 0) {
		buf_packet->len = 1e9;
	}

	bpf_ringbuf_submit(buf_packet, 0);

	return XDP_PASS;
}

char _license[] SEC("license") = "GPL";
