/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	controlplanev1alpha1 "github.com/miscord-dev/toxfu/controlplane/api/v1alpha1"
	toxfulabels "github.com/miscord-dev/toxfu/toxfud/pkg/labels"
	"github.com/miscord-dev/toxfu/toxfuengine"
	"github.com/vishvananda/netlink"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// PeersSyncReconciler reconciles a PeersSync object
type PeersSyncReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	ControlPlaneNamespace string
	ClusterName           string
	NodeName              string
	Engine                toxfuengine.ToxfuEngine

	PrivateKey string
	// ListenPort is immutable field
	ListenPort   int
	STUNEndpoint string
}

//+kubebuilder:rbac:groups=client.miscord.win,resources=peerssyncs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=client.miscord.win,resources=peerssyncs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=client.miscord.win,resources=peerssyncs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PeersSync object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *PeersSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var peers controlplanev1alpha1.PeerNodeList
	err := r.List(ctx, &peers, &client.ListOptions{
		Namespace: r.ControlPlaneNamespace,
	})

	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to list PeerNodes: %w", err)
	}

	selfNodeIndex := -1
	for i, peer := range peers.Items {
		ok := true

		for k, v := range r.labels() {
			if peer.Labels[k] != v {
				ok = false
			}
		}

		if !ok {
			continue
		}

		selfNodeIndex = i
		break
	}

	if selfNodeIndex == -1 {
		return reconcile.Result{}, nil
	}

	// filter out self node

	peers.Items = append(peers.Items[:selfNodeIndex], peers.Items[selfNodeIndex+1:]...)

	peerConfigs := make([]toxfuengine.PeerConfig, 0, len(peers.Items))
	for _, peer := range peers.Items {
		logger := logger.WithValues("peer", peer.Name)

		selector, err := v1.LabelSelectorAsSelector(&peer.Spec.ClaimsSelector)

		if err != nil {
			logger.Error(err, "failed to get selector from claimsSelector")

			continue
		}

		addressesSelector, err := v1.LabelSelectorAsSelector(&peer.Spec.AddressesSelector)

		if err != nil {
			logger.Error(err, "failed to get selector from addressesSelector")

			continue
		}

		var claims controlplanev1alpha1.CIDRClaimList

		err = r.List(ctx, &claims, &client.ListOptions{
			Namespace:     r.ControlPlaneNamespace,
			LabelSelector: selector,
		})

		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to lsit CIDR claims: %w", err)
		}

		pc := toxfuengine.PeerConfig{
			Endpoints:      peer.Spec.Endpoints,
			PublicKey:      peer.Spec.PublicKey,
			PublicDiscoKey: peer.Spec.PublicDiscoKey,
		}

		for _, claim := range claims.Items {
			if claim.Status.State != controlplanev1alpha1.CIDRClaimStatusStateReady {
				continue
			}

			if addressesSelector.Matches(labels.Set(claim.Labels)) {
				pc.Addresses = append(pc.Addresses, claim.Status.CIDR)
			}

			pc.AllowedIPs = append(pc.AllowedIPs, claim.Status.CIDR)
		}

		peerConfigs = append(peerConfigs, pc)
	}

	addrs, err := r.getSelfAddresses(ctx)

	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get self addresses: %w", err)
	}

	r.Engine.Reconfig(&toxfuengine.Config{
		PrivateKey:   r.PrivateKey,
		ListenPort:   r.ListenPort,
		STUNEndpoint: r.STUNEndpoint,
		Addresses:    addrs,
		Peers:        peerConfigs,
	})

	return ctrl.Result{}, nil
}

func (r *PeersSyncReconciler) getSelfAddresses(ctx context.Context) ([]netlink.Addr, error) {
	logger := log.FromContext(ctx)

	var addresses controlplanev1alpha1.CIDRClaimList
	err := r.List(ctx, &addresses, &client.ListOptions{
		Namespace:     r.ControlPlaneNamespace,
		LabelSelector: labels.SelectorFromSet(r.addressLabels()),
	})

	if err != nil {
		return nil, fmt.Errorf("faield to list CIDRClaims for self node: %w", err)
	}

	addrs := make([]netlink.Addr, 0, len(addresses.Items))
	for _, a := range addresses.Items {
		if a.Status.State != controlplanev1alpha1.CIDRClaimStatusStateReady {
			continue
		}

		addr, err := netlink.ParseAddr(a.Status.CIDR)

		if err != nil {
			logger.Error(
				err,
				"failed to parse address",
				"cidrClaim", fmt.Sprintf("%s/%s", a.Namespace, a.Name),
				"address", a.Status.CIDR,
			)

			continue
		}

		addrs = append(addrs, *addr)
	}

	return addrs, nil
}

func (r *PeersSyncReconciler) labels() map[string]string {
	return toxfulabels.ForNode(r.ClusterName, r.NodeName)
}

func (r *PeersSyncReconciler) addressLabels() map[string]string {
	return toxfulabels.NodeTypeForNode(r.ClusterName, r.NodeName)
}

// SetupWithManager sets up the controller with the Manager.
func (r *PeersSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	controlPlaneClaimHandler := handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Name: r.NodeName,
				},
			},
		}
	})

	return ctrl.NewControllerManagedBy(mgr).
		Named("PeersSync").
		Watches(&source.Kind{
			Type: &controlplanev1alpha1.CIDRClaim{},
		}, controlPlaneClaimHandler).
		Watches(&source.Kind{
			Type: &controlplanev1alpha1.PeerNode{},
		}, controlPlaneClaimHandler).
		Complete(r)
}
