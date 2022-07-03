package signalclient

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/miscord-dev/toxfu/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"tailscale.com/types/logger"
)

// RecvCallback is a callback that is called when a message is received.
type RecvCallback func(*proto.NodeRefreshResponse)

var (
	// ErrConnectionNotEstablished is returned when the client or stream is not established.
	ErrConnectionNotEstablished = fmt.Errorf("connection is not established")
)

// Client is a client for the signalling server.
type Client interface {
	// Start starts the client.
	Start()

	// Stop shuts down the client.
	Stop()

	// Close closes all connections and waits for the client to stop after Stop().
	Close() error

	// Send sends a message.
	Send(*proto.NodeRefreshRequest) error

	// RegisterRecvCallback registers a callback to be called when a message is received.
	RegisterRecvCallback(RecvCallback)
}

type signalClient struct {
	conn   io.Closer
	client proto.NodeAPIClient
	logger logger.Logf

	// To stop Refresh()
	ctx    context.Context
	cancel func()

	recvCallbackLock sync.Mutex
	recvCallback     RecvCallback

	sendClientLock sync.Mutex
	sendClient     proto.NodeAPI_RefreshClient

	wg sync.WaitGroup
}

// New creates a new SignalClient.
func New(ctx context.Context, target string, logger logger.Logf) (Client, error) {
	conn, err := grpc.DialContext(
		ctx,
		target,
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                2 * time.Second,
			Timeout:             2 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithInsecure(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	if err != nil {
		return nil, err
	}

	return new(conn, proto.NewNodeAPIClient(conn), logger), nil
}

func new(conn io.Closer, client proto.NodeAPIClient, logger logger.Logf) Client {
	c, cancel := context.WithCancel(context.Background())

	return &signalClient{
		conn:   conn,
		client: client,
		logger: logger,
		ctx:    c,
		cancel: cancel,
	}
}

// Start starts the client.
func (s *signalClient) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.run()
	}()
}

func (s *signalClient) run() {
	for {
		s.callRefresh()

		if s.stopped() {
			return
		}

		time.Sleep(time.Second)
	}
}

func (s *signalClient) callRefresh() {
	it, err := s.client.Refresh(s.ctx)

	if err != nil {
		s.logger("[error] failed to refresh: %v", err)

		return
	}

	s.sendClientLock.Lock()
	s.sendClient = it
	s.sendClientLock.Unlock()

	// recvHandler blocks until the stream is closed.
	s.recvHandler(it)

	s.sendClientLock.Lock()
	s.sendClient = nil
	s.sendClientLock.Unlock()
}

func (s *signalClient) recvHandler(client proto.NodeAPI_RefreshClient) {
	for {
		resp, err := client.Recv()

		if s.stopped() {
			return
		}

		if err != nil {
			s.logger("[error] failed to receive: %v", err)

			return
		}

		s.recvCallbackLock.Lock()
		cb := s.recvCallback
		s.recvCallbackLock.Unlock()

		if cb != nil {
			cb(resp)
		}
	}
}

func (s *signalClient) stopped() bool {
	select {
	case <-s.ctx.Done():
		return true
	default:
		return false
	}
}

// Stop shuts down the client.
func (s *signalClient) Stop() {
	s.cancel()
}

// Wait waits for the client to stop after Stop().
func (s *signalClient) Close() error {
	s.cancel()
	s.wg.Wait()
	return s.conn.Close()
}

// Send sends a message.
func (s *signalClient) Send(req *proto.NodeRefreshRequest) error {
	s.sendClientLock.Lock()
	defer s.sendClientLock.Unlock()

	if s.sendClient == nil {
		return ErrConnectionNotEstablished
	}

	if err := s.sendClient.Send(req); err != nil {
		return err
	}

	return nil
}

// RegisterRecvCallback registers a callback to be called when a message is received.
func (s *signalClient) RegisterRecvCallback(cb RecvCallback) {
	s.recvCallbackLock.Lock()
	defer s.recvCallbackLock.Unlock()

	s.recvCallback = cb
}
