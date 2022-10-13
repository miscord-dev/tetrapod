package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	controlplanev1alpha1 "github.com/miscord-dev/toxfu/controlplane/api/v1alpha1"
)

var _ = Describe("PeerNode", func() {
	ctx, cancel := context.WithCancel(context.Background())

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		err := k8sClient.DeleteAllOf(ctx, &controlplanev1alpha1.CIDRBlock{}, client.InNamespace(testNamespace))
		Expect(err).NotTo(HaveOccurred())

		err = k8sClient.DeleteAllOf(ctx, &controlplanev1alpha1.CIDRClaim{}, client.InNamespace(testNamespace))
		Expect(err).NotTo(HaveOccurred())

		err = k8sClient.DeleteAllOf(ctx, &controlplanev1alpha1.PeerNode{}, client.InNamespace(testNamespace))
		Expect(err).NotTo(HaveOccurred())

		scheme := scheme.Scheme

		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: scheme,
		})
		Expect(err).ToNot(HaveOccurred())

		reconciler := PeerNodeReconciler{
			Client: k8sClient,
			Scheme: scheme,
		}
		err = reconciler.SetupWithManager(mgr)
		Expect(err).NotTo(HaveOccurred())

		go func() {
			err := mgr.Start(ctx)

			if err != nil {
				panic(err)
			}
		}()
		time.Sleep(100 * time.Millisecond)
	})

	AfterEach(func() {
		cancel()

		time.Sleep(100 * time.Millisecond)
	})

	It("cidr-claim", func() {
		peerNode := controlplanev1alpha1.PeerNode{
			ObjectMeta: v1.ObjectMeta{
				Name:      "cidr-block-001-too-small",
				Namespace: testNamespace,
				Labels: map[string]string{
					"controlplane.miscord.win/address-type": "v4",
				},
			},
			Spec: controlplanev1alpha1.PeerNodeSpec{
				CIDRClaims: []controlplanev1alpha1.PeerNodeSpecCIDRClaim{
					{
						Name: "claim001",
						Selector: v1.LabelSelector{
							MatchLabels: map[string]string{
								"key": "label",
							},
						},
						SizeBit: 1,
					},
				},
			},
		}

		err := k8sClient.Create(ctx, &peerNode)
		Expect(err).NotTo(HaveOccurred())

		var cidrClaim controlplanev1alpha1.CIDRClaim
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{
				Namespace: testNamespace,
				Name:      fmt.Sprintf("%s-%s", peerNode.Name, "claim001"),
			}, &cidrClaim)
		}).Should(Succeed())

		Expect(cidrClaim.Spec.SizeBit).To(Equal(peerNode.Spec.CIDRClaims[0].SizeBit))
		Expect(cidrClaim.Spec.Selector.String()).To(Equal(peerNode.Spec.CIDRClaims[0].Selector.String()))

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&peerNode), &peerNode)).ToNot(HaveOccurred())

		Expect(peerNode.Status.State).To(Equal(controlplanev1alpha1.PeerNodeStatusStateUpdating))
		Expect(peerNode.Status.ObservedGeneration).To(Equal(peerNode.Generation))
		Expect(peerNode.Status.Message).To(Equal(""))
		Expect(peerNode.Status.CIDRClaims).To(Equal([]controlplanev1alpha1.PeerNodeStatusCIDRClaim{
			{
				Name:    "claim001",
				Ready:   false,
				Message: "",
				CIDR:    "",
				SizeBit: 0,
			},
		}))

		cidrClaimUpdated := cidrClaim.DeepCopy()
		cidrClaimUpdated.Status.State = controlplanev1alpha1.CIDRClaimStatusStateReady
		cidrClaimUpdated.Status.CIDRBlockName = "cidrblock"
		cidrClaimUpdated.Status.CIDR = "192.168.1.0/24"
		cidrClaimUpdated.Status.SizeBit = 1
		json.NewEncoder(os.Stdout).Encode(cidrClaimUpdated)

		Expect(k8sClient.Status().Update(ctx, cidrClaimUpdated)).ToNot(HaveOccurred())

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&peerNode), &peerNode)).NotTo(HaveOccurred())

			g.Expect(peerNode.Status.State).To(Equal(controlplanev1alpha1.PeerNodeStatusStateReady))
			g.Expect(peerNode.Status.Message).To(Equal(""))
			g.Expect(peerNode.Status.CIDRClaims).To(Equal([]controlplanev1alpha1.PeerNodeStatusCIDRClaim{
				{
					Name:    "claim001",
					Ready:   true,
					Message: "",
					CIDR:    "192.168.1.0/24",
					SizeBit: 1,
				},
			}))
		}).Should(Succeed())
	})
})
