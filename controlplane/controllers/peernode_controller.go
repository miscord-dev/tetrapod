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

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	controlplanev1alpha1 "github.com/miscord-dev/toxfu/controlplane/api/v1alpha1"
)

// PeerNodeReconciler reconciles a PeerNode object
type PeerNodeReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const cidrClaimOwnerPeerNodeKey = "controlplane.miscord.win/owner-peer-node"

//+kubebuilder:rbac:groups=controlplane.miscord.win,resources=peernodes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=controlplane.miscord.win,resources=peernodes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=controlplane.miscord.win,resources=peernodes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PeerNode object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *PeerNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	var peerNode controlplanev1alpha1.PeerNode
	if err := r.Get(ctx, req.NamespacedName, &peerNode); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get PeerNode: %w", err)
	}

	prevCIDRClaims := map[string]controlplanev1alpha1.PeerNodeStatusCIDRClaim{}
	for _, claim := range peerNode.Status.CIDRClaims {
		prevCIDRClaims[claim.Name] = claim
	}

	status := peerNode.Status.DeepCopy()
	status.CIDRClaims = nil

	ready := true
	hasError := false
	for _, claim := range peerNode.Spec.CIDRClaims {
		cidrClaimStatus, err := r.reconcileCIDRClaim(ctx, peerNode, claim, prevCIDRClaims[req.Name])

		if err != nil {
			hasError = true
			ready = false
		}

		if !cidrClaimStatus.Ready {
			ready = false
		}

		status.CIDRClaims = append(status.CIDRClaims, *cidrClaimStatus)
	}

	if ready {
		status.State = controlplanev1alpha1.PeerNodeStatusStateReady
	}
	if hasError {
		status.Message = "upserting claims failed"
	}

	if err := r.updateStatus(ctx, &peerNode, status); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	if err := r.removeUnlinkedClaims(ctx, &peerNode); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to remove unlinked claims: %w", err)
	}

	return ctrl.Result{
		Requeue: hasError,
	}, nil
}

func (r *PeerNodeReconciler) reconcileCIDRClaim(
	ctx context.Context,
	peerNode controlplanev1alpha1.PeerNode,
	claim controlplanev1alpha1.PeerNodeSpecCIDRClaim,
	prevStatus controlplanev1alpha1.PeerNodeStatusCIDRClaim,
) (*controlplanev1alpha1.PeerNodeStatusCIDRClaim, error) {
	cidrClaimStatus := prevStatus.DeepCopy()

	cidrClaim := &controlplanev1alpha1.CIDRClaim{}
	cidrClaim.Labels[cidrClaimOwnerPeerNodeKey] = peerNode.Name
	cidrClaim.Namespace = peerNode.Namespace
	cidrClaim.Name = fmt.Sprintf("%s-%s", peerNode.Name, claim.Name)

	_, err := ctrl.CreateOrUpdate(ctx, r.Client, cidrClaim, func() error {
		cidrClaim.Spec.Selector = claim.Selector
		cidrClaim.Spec.Size = claim.Size

		return ctrl.SetControllerReference(&peerNode, cidrClaim, r.Scheme)
	})

	cidrClaimStatus.Name = claim.Name

	if err != nil {
		cidrClaimStatus.Ready = false
		cidrClaimStatus.Message = fmt.Sprintf("upserting CIDRClaim failed: %v", err)

		return cidrClaimStatus, err
	}

	cidrClaimStatus.CIDR = cidrClaim.Status.CIDR
	cidrClaimStatus.Size = cidrClaim.Status.Size
	cidrClaimStatus.Ready = cidrClaim.Generation == cidrClaim.Status.ObservedGeneration ||
		cidrClaim.Status.State == controlplanev1alpha1.CIDRClaimStatusStateReady

	return cidrClaimStatus, nil
}

func (r *PeerNodeReconciler) removeUnlinkedClaims(ctx context.Context, peerNode *controlplanev1alpha1.PeerNode) error {
	claimKeys := map[string]struct{}{}
	for _, c := range peerNode.Spec.CIDRClaims {
		claimKeys[c.Name] = struct{}{}
	}

	var claimList controlplanev1alpha1.CIDRClaimList
	err := r.Client.List(ctx, &claimList, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			cidrClaimOwnerPeerNodeKey: peerNode.Name,
		}),
	})

	if err != nil {
		return fmt.Errorf("failed to list claims owned by the node: %w", err)
	}

	for _, claim := range claimList.Items {
		_, linked := claimKeys[claim.Name]

		if linked {
			continue
		}

		if err := r.Client.Delete(ctx, &claim); err != nil {
			return fmt.Errorf("failed to delete %s: %w", claim.Name, err)
		}
	}

	return nil
}

func (r *PeerNodeReconciler) updateStatus(ctx context.Context, peerNode *controlplanev1alpha1.PeerNode, status *controlplanev1alpha1.PeerNodeStatus) error {
	updated := peerNode.DeepCopy()
	updated.Status.ObservedGeneration = peerNode.Generation
	updated.Status.State = status.State
	updated.Status.Message = status.Message
	updated.Status.CIDRClaims = status.CIDRClaims

	if err := r.Client.Status().Patch(ctx, updated, client.MergeFrom(peerNode)); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PeerNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1alpha1.PeerNode{}).
		Owns(&controlplanev1alpha1.CIDRClaim{}).
		Complete(r)
}
