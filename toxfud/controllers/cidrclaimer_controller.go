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
	"github.com/miscord-dev/toxfu/toxfud/pkg/labels"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// CIDRClaimerReconciler reconciles a CIDRClaimer object
type CIDRClaimerReconciler struct {
	client.Client
	ControlPlaneNamespace string
	ClusterName           string
	NodeName              string
	TemplateName          string
	ClaimNameGenerator    func() string
	Labels                func() map[string]string
	AllocatedCallback     func(cidr string)

	Scheme *runtime.Scheme
}

func (r *CIDRClaimerReconciler) labels() map[string]string {
	return labels.ForNode(r.ClusterName, r.NodeName)
}

//+kubebuilder:rbac:groups=client.miscord.win,resources=cidrclaimers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=client.miscord.win,resources=cidrclaimers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=client.miscord.win,resources=cidrclaimers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CIDRClaimer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *CIDRClaimerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	var tmpl controlplanev1alpha1.CIDRClaimTemplate

	err := r.Get(ctx, types.NamespacedName{
		Namespace: r.ControlPlaneNamespace,
		Name:      r.TemplateName,
	}, &tmpl)

	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to find template: %w", err)
	}

	var claim controlplanev1alpha1.CIDRClaim
	claim.Namespace = r.ControlPlaneNamespace
	claim.Name = r.ClaimNameGenerator()

	_, err = ctrl.CreateOrUpdate(ctx, r.Client, &claim, func() error {
		claim.Labels = r.labels()
		claim.Spec.Selector = tmpl.Spec.Selector
		claim.Spec.SizeBit = tmpl.Spec.SizeBit

		return nil
	})

	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to upsert claim: %w", err)
	}

	if claim.Status.ObservedGeneration != claim.Generation {
		return reconcile.Result{}, nil
	}

	if r.AllocatedCallback != nil {
		go r.AllocatedCallback(claim.Status.CIDR)
	}

	return ctrl.Result{}, nil
}

// func (r *CIDRClaimerReconciler) deleteClaims(ctx context.Context, principal string) error {
// 	return r.ControlPlaneClusterClient.GetClient().DeleteAllOf(ctx, &controlplanev1alpha1.CIDRClaim{}, &client.DeleteAllOfOptions{
// 		ListOptions: client.ListOptions{
// 			Namespace:     r.ControlPlaneNamespace,
// 			LabelSelector: labels.SelectorFromSet(r.labels()),
// 		},
// 	})
// }

// func (r *CIDRClaimerReconciler) reconcileClaims(ctx context.Context, status *addrstore.ClaimStatus, principal, templateName string) error {
// 	var template controlplanev1alpha1.CIDRClaimTemplate
// 	err := r.ControlPlaneClusterClient.GetCache().Get(ctx, types.NamespacedName{
// 		Namespace: r.ControlPlaneNamespace,
// 		Name:      templateName,
// 	}, &template)

// 	if errors.IsNotFound(err) {
// 		status.Error = fmt.Sprintf("template %s is not found", templateName)

// 		return nil
// 	}

// 	if err != nil {
// 		return fmt.Errorf("failed to find template: %w", err)
// 	}

// 	var claim controlplanev1alpha1.CIDRClaim

// 	claim.Name = r.claimName(principal)
// 	claim.Namespace = r.ControlPlaneNamespace

// 	_, err = ctrl.CreateOrUpdate(ctx, r.ControlPlaneClusterClient.GetClient(), &claim, func() error {
// 		claim.Spec.Selector = template.Spec.Selector
// 		claim.Spec.SizeBit = template.Spec.SizeBit

// 		return nil
// 	})

// 	if err != nil {
// 		return fmt.Errorf("failed to upsert cidr claim %s: %w", claim.Name, err)
// 	}

// 	switch claim.Status.State {
// 	case controlplanev1alpha1.CIDRClaimStatusStateReady:
// 		status.CIDR = claim.Status.CIDR
// 		status.Ready = true
// 	case controlplanev1alpha1.CIDRClaimStatusStateBindingError:
// 		status.Error = claim.Status.Message
// 	case controlplanev1alpha1.CIDRClaimStatusStateUnknown:
// 	}

// 	return nil
// }

// SetupWithManager sets up the controller with the Manager.
func (r *CIDRClaimerReconciler) SetupWithManager(mgr ctrl.Manager, name string) error {
	ch := make(chan event.GenericEvent, 1)
	ch <- event.GenericEvent{
		Object: &unstructured.Unstructured{
			Object: map[string]any{
				"metadata": map[string]any{
					"name": "example",
				},
			},
		},
	}

	channelHandler := handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Name: o.GetName(),
				},
			},
		}
	})
	controlPlaneClaimHandler := handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
		cidrClaim := o.(*controlplanev1alpha1.CIDRClaim)

		labels := r.labels()

		for k, v := range labels {
			if cidrClaim.Labels[k] != v {
				return nil
			}
		}

		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Name: "example",
				},
			},
		}
	})

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		Watches(&source.Channel{
			Source: ch,
		}, channelHandler).
		Watches(&source.Kind{
			Type: &controlplanev1alpha1.CIDRClaim{},
		}, controlPlaneClaimHandler).
		Complete(r)
}
