package signaling

import (
	"context"
	"fmt"
	"time"

	"github.com/miscord-dev/toxfu/controlplane/api/v1alpha1"
	"github.com/miscord-dev/toxfu/toxfucni/pkg/monitor"
	"github.com/miscord-dev/toxfu/toxfucni/toxfuengine"
	"go.uber.org/zap"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	LabelKeyCluster = "toxfu.miscord.win/cluster"
	LabelKeyNode    = "toxfu.miscord.win/node"
	LabelKeyType    = "toxfu.miscord.win/type"
)

const (
	TypeNodeAddress   = "node-address"
	TypeSubnetRouting = "subnet-routing"
)

type Signaling struct {
	logger          *zap.Logger
	cache           cache.Cache
	client          client.Client
	engine          toxfuengine.ToxfuEngine
	monitor         monitor.Monitor
	signalingConfig *SignalingConfig

	closed chan struct{}
}

type SignalingConfig struct {
	ClusterName string
	NodeName    string
	Namespace   string
	ToxfuConfig toxfuengine.Config

	// attributes
	Arch     string
	OS       string
	HostName string
}

func New(
	logger *zap.Logger,
	cache cache.Cache,
	client client.Client,
	engine toxfuengine.ToxfuEngine,
	monitor monitor.Monitor,
	signalingConfig *SignalingConfig,
) (*Signaling, error) {
	s := &Signaling{
		logger:          logger.With(zap.String("component", "signaling")),
		cache:           cache,
		client:          client,
		engine:          engine,
		monitor:         monitor,
		signalingConfig: signalingConfig,
	}

	s.engine.Notify(s.engineCallback)
	go s.runMonitor()

	if err := s.setUpEventHandler(); err != nil {
		return nil, fmt.Errorf("failed to set up event handler: %w", err)
	}

	return s, nil
}

func (s *Signaling) MySelector() map[string]string {
	return map[string]string{
		LabelKeyCluster: s.signalingConfig.ClusterName,
		LabelKeyNode:    s.signalingConfig.NodeName,
	}
}

func (s *Signaling) peerNodeName() string {
	return fmt.Sprintf(
		"%s-%s",
		s.signalingConfig.ClusterName,
		s.signalingConfig.NodeName,
	)
}

func (s *Signaling) runMonitor() {
	subscriber := s.monitor.Subscribe()
	defer subscriber.Close()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-subscriber.C():
		case <-ticker.C:

		case <-s.closed:
			return
		}

		s.engine.Trigger()
	}
}

func (s *Signaling) updatePeerNode(ctx context.Context, pc toxfuengine.PeerConfig) error {
	var peerNode v1alpha1.PeerNode
	peerNode.Name = s.peerNodeName()
	peerNode.Namespace = s.signalingConfig.Namespace

	_, err := ctrl.CreateOrUpdate(ctx, s.client, &peerNode, func() error {
		peerNode.Spec.ClaimsSelector.MatchLabels = s.MySelector()
		peerNode.Spec.Endpoints = pc.Endpoints
		peerNode.Spec.PublicDiscoKey = pc.PublicDiscoKey
		peerNode.Spec.PublicKey = pc.PublicKey
		peerNode.Spec.Attributes.Arch = s.signalingConfig.Arch
		peerNode.Spec.Attributes.HostName = s.signalingConfig.HostName
		peerNode.Spec.Attributes.OS = s.signalingConfig.OS

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to upsert PeerNode: %w", err)
	}

	return nil
}

func (s *Signaling) engineCallback(pc toxfuengine.PeerConfig) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := s.updatePeerNode(ctx, pc); err != nil {
			s.logger.Error("failed to update peer node", zap.Error(err))
		}
	}()
}

func (s *Signaling) setUpEventHandler() error {
	var peerNode v1alpha1.PeerNode
	informer, err := s.cache.GetInformer(context.Background(), &peerNode)

	if err != nil {
		return fmt.Errorf("failed to get informer for PeerNode: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan struct{}, 1)

	go func() {
		<-s.closed
		cancel()
		close(ch)
	}()
	go func() {
		for range ch {
			if err := s.updatePeers(ctx); err != nil {
				s.logger.Error("failed to update peers", zap.Error(err))

				continue
			}
		}
	}()

	informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			select {
			case ch <- struct{}{}:
			default:
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			select {
			case ch <- struct{}{}:
			default:
			}
		},
		DeleteFunc: func(obj interface{}) {
			select {
			case ch <- struct{}{}:
			default:
			}
		},
	})

	return nil
}

func (s *Signaling) updatePeers(ctx context.Context) error {
	var peers v1alpha1.PeerNodeList
	err := s.client.List(ctx, &peers, &client.ListOptions{
		Namespace: s.signalingConfig.Namespace,
	})

	if err != nil {
		return fmt.Errorf("failed to update peers: %w", err)
	}

	peersConfigs := make([]toxfuengine.PeerConfig, 0, len(peers.Items))
	for _, peer := range peers.Items {
		logger := s.logger.With(
			zap.String("namespace", peer.Namespace),
			zap.String("name", peer.Name),
		)

		selector, err := v1.LabelSelectorAsSelector(&peer.Spec.ClaimsSelector)

		if err != nil {
			logger.Error("failed to get selector from claimsSelector", zap.Error(err))

			continue
		}

		opts := &client.ListOptions{
			Namespace:     s.signalingConfig.Namespace,
			LabelSelector: selector,
		}

		var claims v1alpha1.CIDRClaimList
		if err := s.client.List(ctx, &claims, opts); err != nil {
			logger.Error("failed to get selector from claimsSelector", zap.Error(err))

			continue
		}

		addresses := make([]string, 0, len(claims.Items))
		allowedIPs := make([]string, 0, len(claims.Items))

		for _, claim := range claims.Items {
			if claim.Status.State != v1alpha1.CIDRClaimStatusStateReady {
				continue
			}

			allowedIPs = append(allowedIPs, claim.Status.CIDR)

			if claim.Labels[LabelKeyType] == TypeNodeAddress {
				addresses = append(addresses, claim.Status.CIDR)
			}
		}

		peersConfigs = append(peersConfigs, toxfuengine.PeerConfig{
			Endpoints:      peer.Spec.Endpoints,
			PublicKey:      peer.Spec.PublicKey,
			PublicDiscoKey: peer.Spec.PublicDiscoKey,
			Addresses:      addresses,
			AllowedIPs:     allowedIPs,
		})
	}

	toxfuConfig := s.signalingConfig.ToxfuConfig
	toxfuConfig.Peers = peersConfigs

	s.engine.Reconfig(&toxfuConfig)

	return nil
}

func (s *Signaling) Close() error {
	close(s.closed)

	return s.engine.Close()
}
