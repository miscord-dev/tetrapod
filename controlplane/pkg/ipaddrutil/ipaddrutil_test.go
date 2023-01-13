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
			want: []string{
				"fe80::/17",
				"fe80:8000::/18",
				"fe80:c000::/19",
				"fe80:e000::/21",
				"fe80:e800::/22",
				"fe80:ec00::/23",
				"fe80:ee00::/25",
				"fe80:ee80::/26",
				"fe80:eec0::/27",
				"fe80:eee0::/29",
				"fe80:eee8::/30",
				"fe80:eeec::/31",
				"fe80:eeee:0:1::/64",
				"fe80:eeee:0:2::/63",
				"fe80:eeee:0:4::/62",
				"fe80:eeee:0:8::/61",
				"fe80:eeee:0:10::/60",
				"fe80:eeee:0:20::/59",
				"fe80:eeee:0:40::/58",
				"fe80:eeee:0:80::/57",
				"fe80:eeee:0:100::/56",
				"fe80:eeee:0:200::/55",
				"fe80:eeee:0:400::/54",
				"fe80:eeee:0:800::/53",
				"fe80:eeee:0:1000::/52",
				"fe80:eeee:0:2000::/51",
				"fe80:eeee:0:4000::/50",
				"fe80:eeee:0:8000::/49",
				"fe80:eeee:1::/48",
				"fe80:eeee:2::/47",
				"fe80:eeee:4::/46",
				"fe80:eeee:8::/45",
				"fe80:eeee:10::/44",
				"fe80:eeee:20::/43",
				"fe80:eeee:40::/42",
				"fe80:eeee:80::/41",
				"fe80:eeee:100::/40",
				"fe80:eeee:200::/39",
				"fe80:eeee:400::/38",
				"fe80:eeee:800::/37",
				"fe80:eeee:1000::/36",
				"fe80:eeee:2000::/35",
				"fe80:eeee:4000::/34",
				"fe80:eeee:8000::/33",
				"fe80:eeef::/32",
				"fe80:eef0::/28",
				"fe80:ef00::/24",
				"fe80:f000::/21",
				"fe80:f800::/22",
				"fe80:fc00::/23",
				"fe80:fe00::/24",
				"fe80:ff00::/25",
				"fe80:ff80::/26",
				"fe80:ffc0::/27",
				"fe80:ffe0::/28",
				"fe80:fff0::/29",
				"fe80:fff8::/30",
				"fe80:fffc::/31",
				"fe80:fffe::/32",
			},
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
