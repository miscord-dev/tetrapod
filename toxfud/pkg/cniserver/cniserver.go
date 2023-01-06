package cniserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"path/filepath"
	"time"

	"github.com/cenkalti/backoff/v4"
	controlplanev1alpha1 "github.com/miscord-dev/toxfu/controlplane/api/v1alpha1"
	"github.com/miscord-dev/toxfu/toxfud/pkg/labels"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const DefaultSocketPath = "/run/toxfu/cni.sock"

type Options struct {
	Cache                 cache.Cache
	ControlPlaneNamespace string
	ClusterName           string
	NodeName              string
}

type Server interface {
	Run() error
	Shutdown()
}

func NewServer(socketPath string, opt Options) (Server, error) {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0700); err != nil {
		return nil, fmt.Errorf("failed to mkdirall %s: %w", socketPath, err)
	}

	l, err := net.Listen("unix", socketPath)

	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", socketPath, err)
	}

	srv := http.Server{}
	rpcServer := rpc.NewServer()
	rpcServer.Register(&Handler{
		cache:                 opt.Cache,
		controlPlaneNamespace: opt.ControlPlaneNamespace,
		clusterName:           opt.ClusterName,
		nodeName:              opt.NodeName,
	})
	mux := http.NewServeMux()
	mux.Handle(rpc.DefaultRPCPath, rpcServer)

	srv.Handler = mux

	return &server{
		srv: &srv,
		l:   l,
	}, nil
}

type server struct {
	srv *http.Server
	l   net.Listener
}

func (s *server) Run() error {
	return s.srv.Serve(s.l)
}

func (s *server) Shutdown() {
	s.srv.Shutdown(context.TODO())
}

type Handler struct {
	cache                 cache.Cache
	controlPlaneNamespace string
	clusterName           string
	nodeName              string
}

func (h *Handler) newExpBackoff() backoff.BackOff {
	exp := &backoff.ExponentialBackOff{
		InitialInterval:     100 * time.Millisecond,
		RandomizationFactor: backoff.DefaultRandomizationFactor,
		Multiplier:          backoff.DefaultMultiplier,
		MaxInterval:         5 * time.Second,
		MaxElapsedTime:      30 * time.Second,
		Stop:                backoff.Stop,
		Clock:               backoff.SystemClock,
	}
	exp.Reset()

	return exp
}

type GetPodCIDRsArgs struct {
}

func (h *Handler) GetPodCIDRs(args *GetPodCIDRsArgs, cidrClaims *controlplanev1alpha1.CIDRClaimList) error {
	fn := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := h.cache.List(ctx, cidrClaims, &client.ListOptions{
			Namespace:     h.controlPlaneNamespace,
			LabelSelector: k8slabels.SelectorFromSet(labels.NodeTypeForPodCIDR(h.clusterName, h.nodeName)),
		})

		if err != nil {
			return err
		}

		for _, claim := range cidrClaims.Items {
			generationOK := claim.Status.ObservedGeneration == claim.Generation
			statusOK := claim.Status.State == controlplanev1alpha1.CIDRClaimStatusStateReady

			if !generationOK || !statusOK {
				return fmt.Errorf("CIDRClaim %s/%s is not ready", h.controlPlaneNamespace, claim.Name)
			}
		}

		return nil
	}

	err := backoff.Retry(fn, h.newExpBackoff())

	if err != nil {
		return err
	}

	return nil
}

type GetExtraPodCIDRsArgs struct {
	Namespace, Name string
}

func (h *Handler) GetExtraPodCIDRs(args *GetExtraPodCIDRsArgs, cidrClaims *controlplanev1alpha1.CIDRClaimList) error {
	fn := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := h.cache.List(ctx, cidrClaims, &client.ListOptions{
			Namespace:     h.controlPlaneNamespace,
			LabelSelector: k8slabels.SelectorFromSet(labels.NodeTypeForExtraPodCIDR(h.clusterName, h.nodeName, args.Namespace, args.Name)),
		})

		if err != nil {
			return err
		}

		for _, claim := range cidrClaims.Items {
			generationOK := claim.Status.ObservedGeneration == claim.Generation
			statusOK := claim.Status.State == controlplanev1alpha1.CIDRClaimStatusStateReady

			if !generationOK || !statusOK {
				return fmt.Errorf("CIDRClaim %s/%s is not ready", h.controlPlaneNamespace, claim.Name)
			}
		}

		return nil
	}

	err := backoff.Retry(fn, h.newExpBackoff())

	if err != nil {
		return err
	}

	return nil
}
