package signalserver

import (
	"context"
	"time"

	"github.com/miscord-dev/toxfu/persistent"
	"github.com/miscord-dev/toxfu/pkg/syncutil"
	"github.com/miscord-dev/toxfu/proto"
	"github.com/samber/lo"
	protobuf "google.golang.org/protobuf/proto"
	"tailscale.com/types/logger"
)

type Server struct {
	proto.UnimplementedNodeAPIServer

	stunServer string
	log        logger.Logf
	persistent persistent.Persistent
	cond       *syncutil.Cond
}

func NewServer(
	stunServer string,
	log logger.Logf,
	persistent persistent.Persistent,
) proto.NodeAPIServer {
	return &Server{
		stunServer: stunServer,
		log:        log,
		persistent: persistent,
		cond:       syncutil.New(),
	}
}

var _ proto.NodeAPIServer = (*Server)(nil)

func (s *Server) refreshSender(
	ctx context.Context,
	svr proto.NodeAPI_RefreshServer,
	id int64,
	cancel func(),
) {
	sub := s.cond.NewSubscriber()
	defer sub.Close()

	prevResp := &proto.NodeRefreshResponse{}

	for {
		nodes, err := s.persistent.List(ctx)

		if err != nil {
			s.log("failed to list nodes: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		peers := lo.Filter(nodes, func(node *proto.Node, idx int) bool {
			return node.Id != id
		})
		selfNode := lo.Filter(nodes, func(node *proto.Node, idx int) bool {
			return node.Id == id
		})

		if len(selfNode) != 1 {
			s.log("failed to find self node")
			time.Sleep(1 * time.Second)
			continue
		}

		resp := &proto.NodeRefreshResponse{
			SelfNode:   selfNode[0],
			StunServer: s.stunServer,
			Peers:      peers,
		}

		if prevResp != nil && protobuf.Equal(resp, prevResp) {
			continue
		}

		s.log("Sending current status to node(%v): %v", id, resp)

		if err := svr.Send(resp); err != nil {
			s.log("send failed: %v", err)

			cancel()
			return
		}

		select {
		case <-ctx.Done():
		case <-sub.C:
		}

		if isCanceled(ctx) {
			return
		}
	}
}

func (s *Server) refreshReceiver(
	ctx context.Context,
	svr proto.NodeAPI_RefreshServer,
	cancel func(),
) {
	var id int64
	started := false

	for {
		req, err := svr.Recv()

		if isCanceled(ctx) {
			return
		}

		if err != nil {
			s.log("recv failed(%v): %v", id, err)

			cancel()
			return
		}

		s.log("upserting %s", req.PublicKey)
		id, err = s.persistent.Upsert(ctx, req)
		if err != nil {
			s.log("failed to upsert node(%s): %v", req.PublicKey, err)

			continue
		}

		if !started {
			started = true
			go s.refreshSender(ctx, svr, id, cancel)
		}

		s.cond.Broadcast()
	}
}

func (s *Server) Refresh(svr proto.NodeAPI_RefreshServer) error {
	s.log("refresh started")

	ctx, cancel := context.WithCancel(svr.Context())

	go s.refreshReceiver(ctx, svr, cancel)

	<-ctx.Done()

	return nil
}

func isCanceled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
	}

	return false
}
