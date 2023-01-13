package cniserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	controlplanev1alpha1 "github.com/miscord-dev/toxfu/controlplane/api/v1alpha1"
	"github.com/miscord-dev/toxfu/toxfud/pkg/labels"
	corev1 "k8s.io/api/core/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const DefaultSocketPath = "/run/toxfu/cni.sock"

type Options struct {
	Cache                    cache.Cache
	LocalCache               cache.Cache
	ControlPlaneNamespace    string
	ClusterName              string
	NodeName                 string
	PodAddressClaimTemplates []string
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
		cache:                    opt.Cache,
		localCache:               opt.LocalCache,
		controlPlaneNamespace:    opt.ControlPlaneNamespace,
		clusterName:              opt.ClusterName,
		nodeName:                 opt.NodeName,
		podAddressClaimTemplates: opt.PodAddressClaimTemplates,
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
	cache                    cache.Cache
	localCache               cache.Cache
	controlPlaneNamespace    string
	clusterName              string
	nodeName                 string
	podAddressClaimTemplates []string
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
	templateNames := map[string]struct{}{}
	for _, tmpl := range h.podAddressClaimTemplates {
		templateNames[tmpl] = struct{}{}
	}

	fn := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := h.cache.List(ctx, cidrClaims, &client.ListOptions{
			Namespace:     h.controlPlaneNamespace,
			LabelSelector: k8slabels.SelectorFromSet(labels.PodCIDRTypeForNode(h.clusterName, h.nodeName, "")),
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

			templateName := claim.Labels[labels.TemplateNameLabelKey]

			if templateName != "" {
				delete(templateNames, templateName)
			}
		}

		if len(templateNames) != 0 {
			names := []string{}
			for k := range templateNames {
				names = append(names, k)
			}

			return fmt.Errorf("CIDRClaim for %s does not exist", strings.Join(names, ", "))
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

		var pod corev1.Pod
		err := h.localCache.Get(ctx, types.NamespacedName{
			Namespace: args.Namespace,
			Name:      args.Name,
		}, &pod)

		if err != nil {
			return fmt.Errorf("failed to find pod %s/%s: %w", args.Namespace, args.Name, err)
		}

		templateNames := map[string]struct{}{}
		for _, tmpl := range labels.ExtraPODCIDRTemplateNames(pod.Annotations[labels.AnnotationExtraPodCIDRTemplatesKey]) {
			templateNames[tmpl] = struct{}{}
		}

		err = h.cache.List(ctx, cidrClaims, &client.ListOptions{
			Namespace:     h.controlPlaneNamespace,
			LabelSelector: k8slabels.SelectorFromSet(labels.ExtraPodCIDRTypeForNode(h.clusterName, h.nodeName, args.Namespace, args.Name, "")),
		})

		if err != nil {
			return err
		}

		for _, claim := range cidrClaims.Items {
			generationOK := claim.Status.ObservedGeneration == claim.Generation
			statusOK := claim.Status.State == controlplanev1alpha1.CIDRClaimStatusStateReady

			if !generationOK || !statusOK {
				return fmt.Errorf("extra CIDRClaim %s/%s is not ready", h.controlPlaneNamespace, claim.Name)
			}

			delete(templateNames, claim.Labels[labels.TemplateNameLabelKey])
		}

		if len(templateNames) != 0 {
			names := []string{}
			for k := range templateNames {
				names = append(names, k)
			}

			return fmt.Errorf("extra CIDRClaim for %s does not exist", strings.Join(names, ", "))
		}

		return nil
	}

	err := backoff.Retry(fn, h.newExpBackoff())

	if err != nil {
		return err
	}

	return nil
}
