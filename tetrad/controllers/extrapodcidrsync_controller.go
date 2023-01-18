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
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"

	controlplanev1alpha1 "github.com/miscord-dev/tetrapod/controlplane/api/v1alpha1"
	"github.com/miscord-dev/tetrapod/tetrad/pkg/labels"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// ExtraPodCIDRSyncReconciler reconciles a ExtraPodCIDRSync object
type ExtraPodCIDRSyncReconciler struct {
	client.Client
	Scheme                *runtime.Scheme
	ClusterName           string
	NodeName              string
	ControlPlaneNamespace string

	Local cluster.Cluster
}

//+kubebuilder:rbac:groups=client.miscord.win,resources=ExtraPodCIDRSyncs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=client.miscord.win,resources=ExtraPodCIDRSyncs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=client.miscord.win,resources=ExtraPodCIDRSyncs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ExtraPodCIDRSync object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *ExtraPodCIDRSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	var pod corev1.Pod
	err := r.Local.GetClient().Get(ctx, req.NamespacedName, &pod)

	if errors.IsNotFound(err) {
		return ctrl.Result{}, r.deleteCIDRClaim(ctx, req)
	}

	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get Pod: %w", err)
	}

	templateNames := labels.ExtraPODCIDRTemplateNames(pod.Annotations[labels.AnnotationExtraPodCIDRTemplatesKey])

	for _, templateName := range templateNames {
		templateName = strings.TrimSpace(templateName)

		claimName := r.claimName(req.Namespace, req.Name, templateName)

		var tmpl controlplanev1alpha1.CIDRClaimTemplate
		err = r.Get(ctx, types.NamespacedName{
			Namespace: r.ControlPlaneNamespace,
			Name:      templateName,
		}, &tmpl)

		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to find template: %w", err)
		}

		var claim controlplanev1alpha1.CIDRClaim
		claim.Namespace = r.ControlPlaneNamespace
		claim.Name = claimName

		_, err = ctrl.CreateOrUpdate(ctx, r.Client, &claim, func() error {
			claim.Labels = labels.ExtraPodCIDRTypeForNode(r.ClusterName, r.NodeName, req.Namespace, req.Name, templateName)
			claim.Labels[labels.TemplateNameLabelKey] = templateName

			claim.Spec.Selector = tmpl.Spec.Selector
			claim.Spec.SizeBit = tmpl.Spec.SizeBit

			return nil
		})

		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to upsert CIDRClaim: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

func (r *ExtraPodCIDRSyncReconciler) claimName(namespace, name, templateName string) string {
	const maxRawNameLength = 53 - 9

	rawName := fmt.Sprintf(
		"%s/%s/%s/%s/%s",
		r.ClusterName,
		r.NodeName,
		namespace,
		name,
		templateName,
	)

	if len(name) > maxRawNameLength {
		rawName = rawName[:maxRawNameLength]
	}

	hash := sha1.Sum([]byte(name))

	return strings.ReplaceAll(rawName, "/", "-") +
		"-" + hex.EncodeToString(hash[:])[:8]
}

func (r *ExtraPodCIDRSyncReconciler) deleteCIDRClaim(ctx context.Context, req ctrl.Request) error {
	err := r.DeleteAllOf(ctx, &controlplanev1alpha1.CIDRClaim{}, &client.DeleteAllOfOptions{
		ListOptions: client.ListOptions{
			LabelSelector: k8slabels.SelectorFromSet(labels.ExtraPodCIDRTypeForNode(
				r.ClusterName,
				r.NodeName,
				req.Namespace,
				req.Name,
				"",
			)),
			Namespace: r.ControlPlaneNamespace,
		},
	})

	if err != nil {
		return fmt.Errorf("failed to delete CIDRClaims: %w", err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ExtraPodCIDRSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	cidrClaimHandler := handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
		cidrClaim := o.(*controlplanev1alpha1.CIDRClaim)

		for k, v := range labels.ExtraPodCIDRTypeForNodeAll(r.ClusterName, r.NodeName, "") {
			if cidrClaim.Labels[k] != v {
				return nil
			}
		}

		return []reconcile.Request{
			{
				NamespacedName: labels.NamespacedNameFromExtraPodCIDR(cidrClaim.Labels),
			},
		}
	})

	podHandler := handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
		pod := o.(*corev1.Pod)

		if pod.Spec.NodeName != r.NodeName {
			return nil
		}
		if pod.Annotations[labels.AnnotationExtraPodCIDRTemplatesKey] == "" {
			return nil
		}

		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Namespace: pod.Namespace,
					Name:      pod.Name,
				},
			},
		}
	})

	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		// For().
		Named("ExtraPodCIDRSync").
		Watches(&source.Kind{
			Type: &controlplanev1alpha1.CIDRClaim{},
		}, cidrClaimHandler).
		Watches(source.NewKindWithCache(&corev1.Pod{}, r.Local.GetCache()), podHandler).
		Complete(r)
}
