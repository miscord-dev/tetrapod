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
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// CIDRClaimerReconciler reconciles a CIDRClaimer object
type CIDRClaimerReconciler struct {
	client.Client
	ControlPlaneNamespace string
	ClusterName           string
	NodeName              string
	TemplateNames         []string
	ClaimNameGenerator    func(templateName string) string
	Labels                func(templateName string) map[string]string
	AllocatedCallback     func(cidr string)

	Scheme *runtime.Scheme
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
	found := false
	for _, t := range r.TemplateNames {
		if t == req.Name {
			found = true
		}
	}

	if !found {
		err := r.deleteUnusedClaims(ctx, r.Labels(req.Name))

		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to delete CIDRClaims for %s: %w", req.Name, err)
		}

		return reconcile.Result{}, nil
	}

	var tmpl controlplanev1alpha1.CIDRClaimTemplate

	err := r.Get(ctx, types.NamespacedName{
		Namespace: r.ControlPlaneNamespace,
		Name:      req.Name,
	}, &tmpl)

	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to find template: %w", err)
	}

	var claim controlplanev1alpha1.CIDRClaim
	claim.Namespace = r.ControlPlaneNamespace
	claim.Name = r.ClaimNameGenerator(req.Name)

	_, err = ctrl.CreateOrUpdate(ctx, r.Client, &claim, func() error {
		claim.Labels = r.Labels(req.Name)
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

func (r *CIDRClaimerReconciler) deleteUnusedClaims(ctx context.Context, l map[string]string) error {
	labels := k8slabels.SelectorFromSet(l)

	err := r.DeleteAllOf(ctx, &controlplanev1alpha1.CIDRClaim{}, &client.DeleteAllOfOptions{
		ListOptions: client.ListOptions{
			Namespace:     r.ControlPlaneNamespace,
			LabelSelector: labels,
		},
	})

	if err != nil {
		return fmt.Errorf("delete all CIDRClaims of %s in %s: %w", labels.String(), r.ControlPlaneNamespace, err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CIDRClaimerReconciler) SetupWithManager(mgr ctrl.Manager, name string) error {
	templateNames := map[string]struct{}{}
	ch := make(chan event.GenericEvent, len(r.TemplateNames))
	for _, t := range r.TemplateNames {
		_, ok := templateNames[t]

		if ok {
			return fmt.Errorf("%s is duplicated", t)
		}

		ch <- event.GenericEvent{
			Object: &unstructured.Unstructured{
				Object: map[string]any{
					"metadata": map[string]any{
						"name": t,
					},
				},
			},
		}
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

		templateName := cidrClaim.Labels[labels.TemplateNameLabelKey]
		labels := r.Labels(templateName)

		for k, v := range labels {
			if cidrClaim.Labels[k] != v {
				return nil
			}
		}

		if templateName == "" {
			return nil
		}

		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Name: templateName,
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
