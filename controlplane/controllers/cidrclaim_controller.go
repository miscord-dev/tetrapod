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
	"math"
	"math/rand"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	controlplanev1alpha1 "github.com/miscord-dev/toxfu/controlplane/api/v1alpha1"
	"github.com/miscord-dev/toxfu/controlplane/pkg/ipaddrutil"
	"github.com/seancfoley/ipaddress-go/ipaddr"
)

// CIDRClaimReconciler reconciles a CIDRClaim object
type CIDRClaimReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=controlplane.miscord.win,resources=cidrclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=controlplane.miscord.win,resources=cidrclaims/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=controlplane.miscord.win,resources=cidrclaims/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CIDRClaim object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *CIDRClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cidrClaim controlplanev1alpha1.CIDRClaim
	if err := r.Get(ctx, req.NamespacedName, &cidrClaim); err != nil {
		return ctrl.Result{}, err
	}

	if cidrClaim.Generation == cidrClaim.Status.ObservedGeneration &&
		cidrClaim.Status.State == controlplanev1alpha1.CIDRClaimStatusStateReady {
		return ctrl.Result{}, nil
	}

	status := cidrClaim.Status.DeepCopy()

	selector, err := metav1.LabelSelectorAsSelector(&cidrClaim.Spec.Selector)
	if err != nil {
		return ctrl.Result{}, err
	}

	var cidrBlocks controlplanev1alpha1.CIDRBlockList
	if err := r.List(ctx, &cidrBlocks, &client.ListOptions{
		Namespace:     req.Namespace,
		LabelSelector: selector,
	}); err != nil {
		return ctrl.Result{}, err
	}

	var cidrClaims controlplanev1alpha1.CIDRClaimList
	if err := r.List(ctx, &cidrClaims, &client.ListOptions{
		Namespace:     req.Namespace,
		LabelSelector: selector,
		FieldSelector: fields.OneTermNotEqualSelector("metadata.name", cidrClaim.Name),
	}); err != nil {
		return ctrl.Result{}, err
	}

	claims := map[string][]controlplanev1alpha1.CIDRClaim{}
	for _, claim := range cidrClaims.Items {
		if claim.Status.CIDRBlockName == "" {
			continue
		}

		claims[claim.Status.CIDRBlockName] = append(claims[claim.Status.CIDRBlockName], claim)
	}

	items := cidrBlocks.Items
	if len(items) == 0 {
		status.State = controlplanev1alpha1.CIDRClaimStatusStateBindingError
		status.Message = "no matching CIDRBlock"

		return ctrl.Result{Requeue: true}, r.updateStatus(ctx, &cidrClaim, status)
	}

	rand.Shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})

	block, allocated, err := r.allocate(&cidrClaim, cidrBlocks.Items, claims)

	if err != nil {
		status.State = controlplanev1alpha1.CIDRClaimStatusStateBindingError
		status.Message = err.Error()

		return ctrl.Result{}, r.updateStatus(ctx, &cidrClaim, status)
	}

	status.State = controlplanev1alpha1.CIDRClaimStatusStateReady
	status.Message = ""
	status.CIDR = allocated
	status.CIDRBlockName = block
	status.Size = cidrClaim.Spec.Size

	return ctrl.Result{}, nil
}

func (r *CIDRClaimReconciler) allocate(
	cidrClaim *controlplanev1alpha1.CIDRClaim,
	blocks []controlplanev1alpha1.CIDRBlock,
	usedClaims map[string][]controlplanev1alpha1.CIDRClaim,
) (cidrBlockName, cidr string, err error) {
	sizeBit := int(math.Log2(float64(cidrClaim.Spec.Size)))

	for _, block := range blocks {
		blockSubnet := ipaddr.NewIPAddressString(block.Spec.CIDR).GetAddress()

		used := []*ipaddr.IPAddress{}
		for _, claim := range usedClaims[block.Name] {
			addr := ipaddr.NewIPAddressString(claim.Status.CIDR).GetAddress()

			if addr == nil {
				continue
			}

			used = append(used, addr)
		}

		allocated := ipaddrutil.FindSubBlock(
			ipaddrutil.FreeBlocks(blockSubnet, used),
			sizeBit,
		)

		if allocated == nil {
			continue
		}

		return block.Name, allocated.String(), nil
	}

	return "", "", fmt.Errorf("no available CIDRBlock")
}

func (r *CIDRClaimReconciler) updateStatus(ctx context.Context, cidrClaim *controlplanev1alpha1.CIDRClaim, status *controlplanev1alpha1.CIDRClaimStatus) error {
	updated := cidrClaim.DeepCopy()
	updated.Status.ObservedGeneration = cidrClaim.Generation
	updated.Status.CIDR = status.CIDR
	updated.Status.Size = status.Size
	updated.Status.State = status.State

	if err := r.Client.Status().Patch(ctx, updated, client.MergeFrom(cidrClaim)); err != nil {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CIDRClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1alpha1.CIDRClaim{}).
		Complete(r)
}
