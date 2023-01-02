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
	goruntime "runtime"
	"sync/atomic"

	"github.com/miscord-dev/toxfu/cnidaemon/pkg/labels"
	"github.com/miscord-dev/toxfu/toxfuengine"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	controlplanev1alpha1 "github.com/miscord-dev/toxfu/controlplane/api/v1alpha1"
)

// PeerNodeSyncReconciler reconciles a PeerNodeSync object
type PeerNodeSyncReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	ControlPlaneNamespace string
	ClusterName           string
	NodeName              string
	Engine                toxfuengine.ToxfuEngine

	peerConfig atomic.Pointer[toxfuengine.PeerConfig]
}

//+kubebuilder:rbac:groups=client.miscord.win,resources=peernodesyncs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=client.miscord.win,resources=peernodesyncs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=client.miscord.win,resources=peernodesyncs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PeerNodeSync object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *PeerNodeSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	peerConfig := r.peerConfig.Load()

	if peerConfig == nil {
		return ctrl.Result{}, nil
	}

	var peerNode controlplanev1alpha1.PeerNode
	peerNode.Namespace = r.ControlPlaneNamespace
	peerNode.Name = fmt.Sprintf("%s-%s", r.ClusterName, r.NodeName)

	_, err := ctrl.CreateOrUpdate(ctx, r.Client, &peerNode, func() error {
		peerNode.Labels = r.labels()

		peerNode.Spec.ClaimsSelector = v1.LabelSelector{
			MatchLabels: r.labels(),
		}
		peerNode.Spec.AddressesSelector = v1.LabelSelector{
			MatchLabels: r.addrsLabels(),
		}

		peerNode.Spec.Endpoints = peerConfig.Endpoints
		peerNode.Spec.PublicDiscoKey = peerConfig.PublicDiscoKey
		peerNode.Spec.PublicKey = peerConfig.PublicKey
		peerNode.Spec.Attributes.Arch = goruntime.GOARCH
		peerNode.Spec.Attributes.OS = goruntime.GOOS
		peerNode.Spec.Attributes.HostName = r.NodeName

		return nil
	})

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to upsert PeerNode %s/%s: %w", peerNode.Namespace, peerNode.Name, err)
	}

	return ctrl.Result{}, nil
}

func (r *PeerNodeSyncReconciler) labels() map[string]string {
	return labels.ForNode(r.ClusterName, r.NodeName)
}

func (r *PeerNodeSyncReconciler) addrsLabels() map[string]string {
	return labels.NodeTypeForNode(r.ClusterName, r.NodeName)
}

// SetupWithManager sets up the controller with the Manager.
func (r *PeerNodeSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ch := make(chan event.GenericEvent, 1)

	r.Engine.Notify(func(pc toxfuengine.PeerConfig) {
		r.peerConfig.Store(&pc)

		select {
		case ch <- event.GenericEvent{}:
		default:
		}
	})

	channelHandler := handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Name: "self-node",
				},
			},
		}
	})

	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		Named("PeerNodesSync").
		Watches(&source.Channel{
			Source: ch,
		}, channelHandler).
		Complete(r)
}
