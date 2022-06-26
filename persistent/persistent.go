package persistent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/miscord-dev/toxfu/persistent/ent"
	"github.com/miscord-dev/toxfu/persistent/ent/address"
	"github.com/miscord-dev/toxfu/persistent/ent/entutil"
	"github.com/miscord-dev/toxfu/persistent/ent/node"
	"github.com/miscord-dev/toxfu/persistent/ent/route"
	"github.com/miscord-dev/toxfu/proto"
	"golang.org/x/exp/slices"
	"inet.af/netaddr"
)

var (
	ErrNodeDisabled = fmt.Errorf("node is disabled")
)

type Persistent interface {
	Upsert(ctx context.Context, req *proto.NodeRefreshRequest) (int64, error)
	List(ctx context.Context) ([]*proto.Node, error)
	AddRoute(ctx context.Context, id int64, route *proto.IPPrefix) (ret *ent.Route, err error)
	DeleteRoute(ctx context.Context, id int64, prefix *proto.IPPrefix) error
	SetStatus(ctx context.Context, id int64, enabled bool) error
}

type entPersistent struct {
	prefix           netaddr.IPPrefix
	client           *ent.Client
	offlineThreshold time.Duration
}

var _ Persistent = (*entPersistent)(nil)

func NewEnt(
	prefix netaddr.IPPrefix,
	client *ent.Client,
	offlineThreshold time.Duration,
) Persistent {
	return &entPersistent{
		prefix:           prefix,
		client:           client,
		offlineThreshold: offlineThreshold,
	}
}

func (p *entPersistent) upsertNode(ctx context.Context, tx *ent.Tx, req *proto.NodeRefreshRequest) (int64, error) {
	entity, err := tx.Node.Query().Where(node.PublicKeyEQ(req.PublicKey)).First(ctx)

	if _, ok := err.(*ent.NotFoundError); err != nil && !ok {
		return 0, fmt.Errorf("failed to find the node: %w", err)
	}

	if entity != nil && entity.State == node.StateDisabled {
		return entity.ID, ErrNodeDisabled
	}

	if entity == nil {
		id, err := tx.Node.
			Create().
			SetPublicKey(req.PublicKey).
			SetPublicDiscoKey(req.PublicDiscoKey).
			SetHostName(req.Attribute.HostName).
			SetOs(req.Attribute.Os).
			SetGoos(req.Attribute.Goos).
			SetEndpoints(req.Endpoints).
			SetGoarch(req.Attribute.Goarch).
			SetLastUpdatedAt(time.Now()).
			SetState(node.StateOnline).
			OnConflict(
				sql.ConflictColumns(node.FieldPublicKey),
			).
			UpdateNewValues().
			ID(ctx)

		if err != nil {
			return 0, fmt.Errorf("failed to create node: %w", err)
		}

		return id, nil
	}

	_, err = tx.Node.Update().
		Where(node.ID(entity.ID)).
		SetPublicKey(req.PublicKey).
		SetPublicDiscoKey(req.PublicDiscoKey).
		SetHostName(req.Attribute.HostName).
		SetOs(req.Attribute.Os).
		SetGoos(req.Attribute.Goos).
		SetEndpoints(req.Endpoints).
		SetGoarch(req.Attribute.Goarch).
		SetLastUpdatedAt(time.Now()).
		SetState(node.StateOnline).
		Save(ctx)

	if err != nil {
		return 0, fmt.Errorf("failed to upsert node: %w", err)
	}

	return entity.ID, nil
}

func (p *entPersistent) Upsert(ctx context.Context, req *proto.NodeRefreshRequest) (id int64, err error) {
	err = entutil.WithTx(ctx, p.client, func(tx *ent.Tx) error {
		id, err = p.upsertNode(ctx, tx, req)

		if err != nil {
			return fmt.Errorf("failed to upsert node: %w", err)
		}

		assignAddress := func() error {
			addrs, err := tx.Address.Query().
				WithHost(func(query *ent.NodeQuery) {
					query.Select(address.FieldID)
				}).
				All(ctx)

			if err != nil {
				return fmt.Errorf("failed to load assigned addresses: %w", err)
			}

			addrStrings := make([]string, 0, len(addrs))
			for _, a := range addrs {
				if a.Edges.Host.ID == id {
					return nil
				}
				addrStrings = append(addrStrings, a.Addr)
			}
			slices.Sort(addrStrings)

			ipRange := p.prefix.Range()
			assigned := ipRange.From().Next()
			for ; assigned.Compare(ipRange.To()) != 0; assigned = assigned.Next() {
				_, contains := slices.BinarySearch(addrStrings, assigned.String())

				if !contains {
					break
				}
			}

			if assigned.Compare(ipRange.To()) == 0 {
				return fmt.Errorf("no available address")
			}

			_, err = tx.Address.Create().
				SetAddr(assigned.String()).
				SetHostID(id).
				Save(ctx)

			if err != nil {
				return fmt.Errorf("failed to save an assigned address: %w", err)
			}

			return nil
		}

		for i := 0; i < 10; i++ {
			err = assignAddress()

			if err == nil {
				return nil
			}
		}

		return fmt.Errorf("failed to assign an address: %w", err)
	})

	return id, err
}

func (p *entPersistent) List(ctx context.Context) ([]*proto.Node, error) {
	nodes, err := p.client.Node.Query().Where(
		node.LastUpdatedAtGT(time.Now().Add(-p.offlineThreshold)),
		node.StateEQ(node.StateOnline),
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

		node.AdvertisedPrefixes = node.Addresses
		for _, r := range n.Edges.Routes {
			node.AdvertisedPrefixes = append(node.AdvertisedPrefixes, &proto.IPPrefix{
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

func (p *entPersistent) SetStatus(ctx context.Context, id int64, enabled bool) error {
	state := node.StateDisabled

	if enabled {
		state = node.StateOffline
	}

	affected, err := p.client.Node.Update().
		SetState(state).
		Where(node.ID(id)).
		Save(ctx)

	if err != nil {
		return fmt.Errorf("failed to disable node: %w", err)
	}

	if affected == 0 {
		return fmt.Errorf("node not found")
	}

	return nil
}

func (p *entPersistent) Close() error {
	return p.client.Close()
}
