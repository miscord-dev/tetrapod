package ipaddrutil

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/seancfoley/ipaddress-go/ipaddr"
)

func addrSliceToStrSlice(slice []*ipaddr.IPAddress) []string {
	s := make([]string, 0, len(slice))
	for _, addr := range slice {
		s = append(s, addr.String())
	}

	return s
}

func ipAddress(s string) *ipaddr.IPAddress {
	return ipaddr.NewIPAddressString(s).GetAddress()
}

func TestFreeBlocks(t *testing.T) {
	type args struct {
		base *ipaddr.IPAddress
		used []*ipaddr.IPAddress
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "192.168.1.0/24",
			args: args{
				base: ipAddress("192.168.1.0/24"),
				used: []*ipaddr.IPAddress{
					ipAddress("192.168.1.0/25"),
					ipAddress("192.168.1.192/26"),
				},
			},
			want: []string{
				"192.168.1.128/26",
			},
		},
		{
			name: "/32",
			args: args{
				base: ipAddress("192.168.1.0/24"),
				used: []*ipaddr.IPAddress{
					ipAddress("192.168.1.0"),
					ipAddress("192.168.1.1"),
					ipAddress("192.168.1.2"),
					ipAddress("192.168.1.3"),
					ipAddress("192.168.1.4"),
					ipAddress("192.168.1.5"),
				},
			},
			want: []string{
				"192.168.1.6/31",
				"192.168.1.8/29",
				"192.168.1.16/28",
				"192.168.1.32/27",
				"192.168.1.64/26",
				"192.168.1.128/25",
			},
		},
		{
			name: "IPv6",
			args: args{
				base: ipAddress("fe80::/16"),
				used: []*ipaddr.IPAddress{
					ipAddress("fe80:eeee::/64"),
					ipAddress("fe80:ffff::/32"),
				},
			},
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FreeBlocks(tt.args.base, tt.args.used)

			if diff := cmp.Diff(
				tt.want,
				addrSliceToStrSlice(got),
			); diff != "" {
				t.Errorf("FreeBlocks() diff %v", diff)
			}

			if tt.args.base.IsIPv4() {
				trie := ipaddr.NewIPv4AddressTrie()

				for _, addr := range got {
					trie.Add((*ipaddr.IPv4Address)(addr))
				}

				t.Log(trie)
			} else if tt.args.base.IsIPv6() {
				trie := ipaddr.NewIPv6AddressTrie()

				for _, addr := range got {
					trie.Add((*ipaddr.IPv6Address)(addr))
				}

				t.Log(trie)
			}
		})
	}
}

func TestFindSubBlock(t *testing.T) {
	type args struct {
		blocks  []*ipaddr.IPAddress
		sizeBit int
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "v4",
			args: args{
				blocks: []*ipaddr.IPAddress{
					ipAddress("192.168.1.6/31"),
					ipAddress("192.168.1.8/29"),
					ipAddress("192.168.1.16/28"),
					ipAddress("192.168.1.32/27"),
					ipAddress("192.168.1.64/26"),
					ipAddress("192.168.1.128/25"),
				},
				sizeBit: 4,
			},
			want: "192.168.1.16/28",
		},
		{
			name: "v4 pick smaller",
			args: args{
				blocks: []*ipaddr.IPAddress{
					ipAddress("192.168.1.0/24"),
				},
				sizeBit: 4,
			},
			want: "192.168.1.0/28",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindSubBlock(tt.args.blocks, tt.args.sizeBit)

			if fmt.Sprint(got) != tt.want {
				t.Errorf("FindSubBlock() = %v, want %v", got, tt.want)
			}
		})
	}
}
