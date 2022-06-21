package persistent

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	_ "github.com/mattn/go-sqlite3"
	"github.com/miscord-dev/toxfu/persistent/ent"
	"github.com/miscord-dev/toxfu/persistent/ent/enttest"
	"github.com/miscord-dev/toxfu/persistent/ent/migrate"
	"github.com/miscord-dev/toxfu/proto"
	"inet.af/netaddr"
)

var (
	testNodeReq = []*proto.NodeRefreshRequest{
		{
			PublicKey:      "public_key",
			PublicDiscoKey: "public_disco_key",
			Endpoints: []string{
				"192.0.2.1:12345",
				"198.51.100.1:12345",
				"203.0.113.1:12345",
			},
			Attribute: &proto.NodeAttribute{
				HostName: "node1",
				Os:       "Test Linux",
				Goos:     "linux",
				Goarch:   "amd64",
			},
		},
		{
			PublicKey:      "public_key2",
			PublicDiscoKey: "public_disco_key2",
			Endpoints: []string{
				"192.0.2.1:12346",
				"198.51.100.1:12346",
				"203.0.113.1:12346",
			},
			Attribute: &proto.NodeAttribute{
				HostName: "node2",
				Os:       "Test Linux",
				Goos:     "linux",
				Goarch:   "amd64",
			},
		},
	}

	testRoute = &proto.IPPrefix{
		Address: "10.1.1.0",
		Bits:    24,
	}

	subnetPrefix = netaddr.MustParseIPPrefix("192.168.1.0/24")

	offlineThreshold = 10 * time.Second
)

func ignoreProtoUnexported() cmp.Option {
	return cmpopts.IgnoreUnexported(proto.Node{}, proto.IPPrefix{}, proto.NodeAttribute{})
}

func TestInsert(t *testing.T) {
	opts := []enttest.Option{
		enttest.WithOptions(ent.Log(t.Log)),
		enttest.WithMigrateOptions(migrate.WithGlobalUniqueID(true)),
	}

	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1", opts...).Debug()
	defer client.Close()

	ctx := context.Background()

	persistent := NewEnt(subnetPrefix, client, offlineThreshold)

	for i, req := range testNodeReq {
		if err := persistent.Upsert(ctx, req); err != nil {
			t.Fatal(i, err)
		}
	}

	expected := []*proto.Node{
		{
			PublicKey:      testNodeReq[0].PublicKey,
			PublicDiscoKey: testNodeReq[0].PublicDiscoKey,
			Endpoints:      testNodeReq[0].Endpoints,
			Addresses: []*proto.IPPrefix{
				{
					Address: "192.168.1.1",
					Bits:    32,
				},
			},
			AdvertisedPrefixes: []*proto.IPPrefix{
				{
					Address: "192.168.1.1",
					Bits:    32,
				},
			},
			Attribute: &proto.NodeAttribute{
				HostName: testNodeReq[0].Attribute.HostName,
				Os:       testNodeReq[0].Attribute.Os,
				Goos:     testNodeReq[0].Attribute.Goos,
				Goarch:   testNodeReq[0].Attribute.Goarch,
			},
		},
		{
			PublicKey:      testNodeReq[1].PublicKey,
			PublicDiscoKey: testNodeReq[1].PublicDiscoKey,
			Endpoints:      testNodeReq[1].Endpoints,
			Addresses: []*proto.IPPrefix{
				{
					Address: "192.168.1.2",
					Bits:    32,
				},
			},
			AdvertisedPrefixes: []*proto.IPPrefix{
				{
					Address: "192.168.1.2",
					Bits:    32,
				},
			},
			Attribute: &proto.NodeAttribute{
				HostName: testNodeReq[1].Attribute.HostName,
				Os:       testNodeReq[1].Attribute.Os,
				Goos:     testNodeReq[1].Attribute.Goos,
				Goarch:   testNodeReq[1].Attribute.Goarch,
			},
		},
	}

	t.Run("list", func(t *testing.T) {
		nodes, err := persistent.List(ctx)

		if err != nil {
			t.Fatal(err)
		}

		if len(nodes) != 2 {
			t.Fatalf("expected 2 node, got %d", len(nodes))
		}

		diff := cmp.Diff(
			expected[0],
			nodes[0],
			cmpopts.IgnoreFields(proto.Node{}, "Id"),
			ignoreProtoUnexported(),
		)

		if diff != "" {
			t.Error(diff)
		}
	})

	t.Run("update", func(t *testing.T) {
		testNodeReq[0].Endpoints = append(testNodeReq[0].Endpoints, "192.0.1.1:12345")

		for i, req := range testNodeReq {
			if err := persistent.Upsert(ctx, req); err != nil {
				t.Fatal(i, err)
			}
		}

		nodes, err := persistent.List(ctx)

		if err != nil {
			t.Fatal(err)
		}

		expected[0].Endpoints = append(expected[0].Endpoints, "192.0.1.1:12345")

		diff := cmp.Diff(
			expected[0],
			nodes[0],
			cmpopts.IgnoreFields(proto.Node{}, "Id"),
			ignoreProtoUnexported(),
		)

		if diff != "" {
			t.Error(diff)
		}
	})

	t.Run("add-route", func(t *testing.T) {
		nodes, err := persistent.List(ctx)

		if err != nil {
			t.Fatal(err)
		}

		r, err := persistent.AddRoute(ctx, nodes[0].GetId(), testRoute)

		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(
			&ent.Route{
				Addr: testRoute.Address,
				Bits: int(testRoute.Bits),
			},
			r,
			cmpopts.IgnoreUnexported(ent.Route{}),
			cmpopts.IgnoreFields(ent.Route{}, "Edges", "ID"),
		); diff != "" {
			t.Error(diff)
		}

		nodesWithRoutes, err := persistent.List(ctx)

		if err != nil {
			t.Fatal(err)
		}

		expected[0].AdvertisedPrefixes = append(expected[0].AdvertisedPrefixes, []*proto.IPPrefix{
			{
				Address: testRoute.Address,
				Bits:    testRoute.Bits,
			},
		}...)

		diff := cmp.Diff(
			expected,
			nodesWithRoutes,
			cmpopts.IgnoreFields(proto.Node{}, "Id"),
			ignoreProtoUnexported(),
		)

		if diff != "" {
			t.Error(diff)
		}
	})

}
