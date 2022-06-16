package persistent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/miscord-dev/toxfu/persistent/ent"
	"github.com/miscord-dev/toxfu/persistent/ent/entutil"
	"github.com/miscord-dev/toxfu/persistent/ent/node"
	"github.com/miscord-dev/toxfu/persistent/ent/route"
	"github.com/miscord-dev/toxfu/proto"
	"inet.af/netaddr"
)

type Persistent interface {
	Upsert(ctx context.Context, req *proto.NodeRefreshRequest) error
	List(ctx context.Context) ([]*proto.Node, error)
	AddRoute(ctx context.Context, id int64, route *proto.IPPrefix) (ret *ent.Route, err error)
	DeleteRoute(ctx context.Context, id int64, prefix *proto.IPPrefix) error
}

type entPersistent struct {
	prefix netaddr.IPPrefix
	client *ent.Client
}

var _ Persistent = (*entPersistent)(nil)

func (p *entPersistent) Upsert(ctx context.Context, req *proto.NodeRefreshRequest) error {
	err := entutil.WithTx(ctx, p.client, func(tx *ent.Tx) error {
		id, err := tx.Node.
			Create().
			SetPublicKey(req.PublicKey).
			SetPublicDiscoKey(req.PublicDiscoKey).
			SetHostName(req.Attribute.HostName).
			SetOs(req.Attribute.Os).
			SetGoos(req.Attribute.Goos).
			SetGoarch(req.Attribute.Goarch).
			SetLastUpdatedAt(time.Now()).
			OnConflict(
				sql.ConflictColumns(node.FieldPublicKey),
			).
			UpdateNewValues().
			ID(ctx)

		if err != nil {
			return fmt.Errorf("failed to upsert node: %w", err)
		}

		addrs, err := tx.Address.Query().
			ForUpdate().
			All(ctx)

		for _, a := range addrs {
			if a.Edges.Host.ID == id {
				return nil
			}
		}

		ipRange := p.prefix.Range()
		assigned := ipRange.From()
		for ; assigned.Compare(ipRange.To()) != 0; assigned.Next() {
		}

		_, err = tx.Address.Create().
			SetAddr(assigned.String()).
			SetHostID(id).
			Save(ctx)

		if err != nil {
			return fmt.Errorf("failed to save an assigned address: %w", err)
		}

		return nil
	})

	return err
}

func (p *entPersistent) List(ctx context.Context) ([]*proto.Node, error) {
	nodes, err := p.client.Node.Query().Where(
		node.LastUpdatedAtGT(time.Now().Add(-10 * time.Second)),
	).WithRoutes().WithAddresses().All(ctx)

	if err != nil {
		return nil, err
	}

	results := make([]*proto.Node, 0, len(nodes))
	for _, n := range nodes {
		node := &proto.Node{
			Id:             n.ID,
			PublicKey:      n.PublicKey,
			PublicDiscoKey: n.PublicDiscoKey,
			Endpoints:      n.Endpoints,
			Attribute: &proto.NodeAttribute{
				HostName: n.HostName,
				Os:       n.Os,
				Goos:     n.Goos,
				Goarch:   n.Goarch,
			},
		}

		for _, a := range n.Edges.Addresses {
			bits := 32
			if strings.Contains(a.Addr, ":") {
				bits = 128
			}

			node.Addresses = append(node.Addresses, &proto.IPPrefix{
				Address: a.Addr,
				Bits:    int32(bits),
			})
		}

		for _, r := range n.Edges.Routes {
			node.Addresses = append(node.Addresses, &proto.IPPrefix{
				Address: r.Addr,
				Bits:    int32(r.Bits),
			})
		}

		results = append(results, node)
	}

	return results, nil
}

func (p *entPersistent) AddRoute(ctx context.Context, id int64, route *proto.IPPrefix) (ret *ent.Route, err error) {
	err = entutil.WithTx(ctx, p.client, func(tx *ent.Tx) error {
		node, err := tx.Node.Get(ctx, id)

		if err != nil {
			return fmt.Errorf("failed to find the target node: %w", err)
		}

		ret, err = tx.Route.Create().
			SetHost(node).
			SetAddr(route.Address).
			SetBits(int(route.Bits)).
			Save(ctx)

		if err != nil {
			return fmt.Errorf("failed to create a route: %w", err)
		}

		return nil
	})

	return
}

func (p *entPersistent) DeleteRoute(ctx context.Context, id int64, prefix *proto.IPPrefix) error {
	err := entutil.WithTx(ctx, p.client, func(tx *ent.Tx) error {
		_, err := tx.Route.Delete().Where(
			route.Addr(prefix.Address),
			route.Bits(int(prefix.Bits)),
			route.HasHostWith(node.ID(id)),
		).Exec(ctx)

		if err != nil {
			return fmt.Errorf("failed to create a route: %w", err)
		}

		return nil
	})

	return err
}

func (p *entPersistent) Close() error {
	return p.client.Close()
}
