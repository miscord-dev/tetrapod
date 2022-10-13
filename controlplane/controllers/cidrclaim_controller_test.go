package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	controlplanev1alpha1 "github.com/miscord-dev/toxfu/controlplane/api/v1alpha1"
)

var _ = Describe("CIDRClaim", func() {
	ctx, cancel := context.WithCancel(context.Background())

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		err := k8sClient.DeleteAllOf(ctx, &controlplanev1alpha1.CIDRBlock{}, client.InNamespace(testNamespace))
		Expect(err).NotTo(HaveOccurred())

		err = k8sClient.DeleteAllOf(ctx, &controlplanev1alpha1.CIDRClaim{}, client.InNamespace(testNamespace))
		Expect(err).NotTo(HaveOccurred())

		scheme := scheme.Scheme

		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: scheme,
		})
		Expect(err).ToNot(HaveOccurred())

		reconciler := CIDRClaimReconciler{
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

	It("Allocate IPv4", func() {
		cidrBlockSmall := controlplanev1alpha1.CIDRBlock{
			ObjectMeta: v1.ObjectMeta{
				Name:      "cidr-block-001-too-small",
				Namespace: testNamespace,
				Labels: map[string]string{
					"controlplane.miscord.win/address-type": "v4",
				},
			},
			Spec: controlplanev1alpha1.CIDRBlockSpec{
				CIDR: "172.16.0.0/31",
			},
		}

		err := k8sClient.Create(ctx, &cidrBlockSmall)
		Expect(err).NotTo(HaveOccurred())

		cidrBlock := controlplanev1alpha1.CIDRBlock{
			ObjectMeta: v1.ObjectMeta{
				Name:      "cidr-block-001-big-enough",
				Namespace: testNamespace,
				Labels: map[string]string{
					"controlplane.miscord.win/address-type": "v4",
				},
			},
			Spec: controlplanev1alpha1.CIDRBlockSpec{
				CIDR: "192.168.1.0/24",
			},
		}

		err = k8sClient.Create(ctx, &cidrBlock)
		Expect(err).NotTo(HaveOccurred())

		cidrClaim := controlplanev1alpha1.CIDRClaim{
			ObjectMeta: v1.ObjectMeta{
				Name:      "cidr-claim",
				Namespace: testNamespace,
			},
			Spec: controlplanev1alpha1.CIDRClaimSpec{
				Selector: v1.LabelSelector{
					MatchLabels: map[string]string{
						"controlplane.miscord.win/address-type": "v4",
					},
				},
				SizeBit: 2,
			},
		}

		err = k8sClient.Create(ctx, &cidrClaim)
		Expect(err).NotTo(HaveOccurred())

		cidrClaimKey := client.ObjectKeyFromObject(&cidrClaim)
		Eventually(func() error {
			err := k8sClient.Get(ctx, cidrClaimKey, &cidrClaim)

			if err != nil {
				return err
			}

			if cidrClaim.Status.ObservedGeneration != cidrClaim.Generation {
				return fmt.Errorf("not updated")
			}

			return nil
		}).Should(Succeed())

		Expect(cidrClaim.Status.State).To(Equal(controlplanev1alpha1.CIDRClaimStatusStateReady))
		Expect(cidrClaim.Status.CIDRBlockName).To(Equal(cidrBlock.Name))
		Expect(cidrClaim.Status.CIDR).To(Equal("192.168.1.0/30"))
		Expect(cidrClaim.Status.SizeBit).To(Equal(2))
	})

	It("Allocate IPv6", func() {
		cidrBlockSmall := controlplanev1alpha1.CIDRBlock{
			ObjectMeta: v1.ObjectMeta{
				Name:      "cidr-block-001-too-small",
				Namespace: testNamespace,
				Labels: map[string]string{
					"controlplane.miscord.win/address-type": "v6",
				},
			},
			Spec: controlplanev1alpha1.CIDRBlockSpec{
				CIDR: "fe80::/96",
			},
		}

		err := k8sClient.Create(ctx, &cidrBlockSmall)
		Expect(err).NotTo(HaveOccurred())

		cidrBlock := controlplanev1alpha1.CIDRBlock{
			ObjectMeta: v1.ObjectMeta{
				Name:      "cidr-block-001-big-enough",
				Namespace: testNamespace,
				Labels: map[string]string{
					"controlplane.miscord.win/address-type": "v6",
				},
			},
			Spec: controlplanev1alpha1.CIDRBlockSpec{
				CIDR: "fe80:1::/64",
			},
		}

		err = k8sClient.Create(ctx, &cidrBlock)
		Expect(err).NotTo(HaveOccurred())

		cidrClaim := controlplanev1alpha1.CIDRClaim{
			ObjectMeta: v1.ObjectMeta{
				Name:      "cidr-claim",
				Namespace: testNamespace,
			},
			Spec: controlplanev1alpha1.CIDRClaimSpec{
				Selector: v1.LabelSelector{
					MatchLabels: map[string]string{
						"controlplane.miscord.win/address-type": "v6",
					},
				},
				SizeBit: 48,
			},
		}

		err = k8sClient.Create(ctx, &cidrClaim)
		Expect(err).NotTo(HaveOccurred())

		cidrClaimKey := client.ObjectKeyFromObject(&cidrClaim)
		Eventually(func() error {
			err := k8sClient.Get(ctx, cidrClaimKey, &cidrClaim)

			if err != nil {
				return err
			}

			if cidrClaim.Status.ObservedGeneration != cidrClaim.Generation {
				return fmt.Errorf("not updated")
			}

			return nil
		}).Should(Succeed())

		Expect(cidrClaim.Status.State).To(Equal(controlplanev1alpha1.CIDRClaimStatusStateReady))
		Expect(cidrClaim.Status.CIDRBlockName).To(Equal(cidrBlock.Name))
		Expect(cidrClaim.Status.CIDR).To(Equal("fe80:1::/80"))
		Expect(cidrClaim.Status.SizeBit).To(Equal(48))
	})

	It("No matching block", func() {
		cidrBlockV6 := controlplanev1alpha1.CIDRBlock{
			ObjectMeta: v1.ObjectMeta{
				Name:      "cidr-block-001",
				Namespace: testNamespace,
				Labels: map[string]string{
					"controlplane.miscord.win/address-type": "v6",
				},
			},
			Spec: controlplanev1alpha1.CIDRBlockSpec{
				CIDR: "fe80::/64",
			},
		}

		err := k8sClient.Create(ctx, &cidrBlockV6)
		Expect(err).NotTo(HaveOccurred())

		cidrClaim := controlplanev1alpha1.CIDRClaim{
			ObjectMeta: v1.ObjectMeta{
				Name:      "cidr-claim",
				Namespace: testNamespace,
			},
			Spec: controlplanev1alpha1.CIDRClaimSpec{
				Selector: v1.LabelSelector{
					MatchLabels: map[string]string{
						"controlplane.miscord.win/address-type": "v4",
					},
				},
				SizeBit: 8,
			},
		}

		err = k8sClient.Create(ctx, &cidrClaim)
		Expect(err).NotTo(HaveOccurred())

		cidrClaimKey := client.ObjectKeyFromObject(&cidrClaim)
		Eventually(func() error {
			err := k8sClient.Get(ctx, cidrClaimKey, &cidrClaim)

			if err != nil {
				return err
			}

			if cidrClaim.Status.ObservedGeneration != cidrClaim.Generation {
				return fmt.Errorf("not updated")
			}

			return nil
		}).Should(Succeed())

		Expect(cidrClaim.Status.State).To(Equal(controlplanev1alpha1.CIDRClaimStatusStateBindingError))
		Expect(cidrClaim.Status.CIDRBlockName).To(Equal(""))
		Expect(cidrClaim.Status.CIDR).To(Equal(""))
		Expect(cidrClaim.Status.SizeBit).To(Equal(0))
		Expect(cidrClaim.Status.Message).To(Equal("no matching CIDRBlock"))
	})

	It("No available block", func() {
		cidrBlockV6 := controlplanev1alpha1.CIDRBlock{
			ObjectMeta: v1.ObjectMeta{
				Name:      "cidr-block-001",
				Namespace: testNamespace,
				Labels: map[string]string{
					"controlplane.miscord.win/address-type": "v6",
				},
			},
			Spec: controlplanev1alpha1.CIDRBlockSpec{
				CIDR: "fe80::/64",
			},
		}

		err := k8sClient.Create(ctx, &cidrBlockV6)
		Expect(err).NotTo(HaveOccurred())

		cidrClaim := controlplanev1alpha1.CIDRClaim{
			ObjectMeta: v1.ObjectMeta{
				Name:      "cidr-claim",
				Namespace: testNamespace,
			},
			Spec: controlplanev1alpha1.CIDRClaimSpec{
				Selector: v1.LabelSelector{
					MatchLabels: map[string]string{
						"controlplane.miscord.win/address-type": "v6",
					},
				},
				SizeBit: 64,
			},
		}

		err = k8sClient.Create(ctx, &cidrClaim)
		Expect(err).NotTo(HaveOccurred())

		cidrClaimKey := client.ObjectKeyFromObject(&cidrClaim)
		Eventually(func() error {
			err := k8sClient.Get(ctx, cidrClaimKey, &cidrClaim)

			if err != nil {
				return err
			}

			if cidrClaim.Status.ObservedGeneration != cidrClaim.Generation {
				return fmt.Errorf("not updated")
			}

			return nil
		}).Should(Succeed())

		Expect(cidrClaim.Status.State).To(Equal(controlplanev1alpha1.CIDRClaimStatusStateReady))
		Expect(cidrClaim.Status.CIDRBlockName).To(Equal("cidr-block-001"))
		Expect(cidrClaim.Status.CIDR).To(Equal("fe80::/64"))
		Expect(cidrClaim.Status.SizeBit).To(Equal(64))
		Expect(cidrClaim.Status.Message).To(Equal(""))

		cidrClaim = controlplanev1alpha1.CIDRClaim{
			ObjectMeta: v1.ObjectMeta{
				Name:      "cidr-claim-2",
				Namespace: testNamespace,
			},
			Spec: controlplanev1alpha1.CIDRClaimSpec{
				Selector: v1.LabelSelector{
					MatchLabels: map[string]string{
						"controlplane.miscord.win/address-type": "v6",
					},
				},
				SizeBit: 64,
			},
		}

		err = k8sClient.Create(ctx, &cidrClaim)
		Expect(err).NotTo(HaveOccurred())

		cidrClaimKey = client.ObjectKeyFromObject(&cidrClaim)
		Eventually(func() error {
			err := k8sClient.Get(ctx, cidrClaimKey, &cidrClaim)

			if err != nil {
				return err
			}

			if cidrClaim.Status.ObservedGeneration != cidrClaim.Generation {
				return fmt.Errorf("not updated")
			}

			return nil
		}).Should(Succeed())

		Expect(cidrClaim.Status.CIDR).To(Equal(""))
		Expect(cidrClaim.Status.CIDRBlockName).To(Equal(""))
		Expect(cidrClaim.Status.SizeBit).To(Equal(0))
		Expect(cidrClaim.Status.Message).To(Equal("no available CIDRBlock"))
		Expect(cidrClaim.Status.State).To(Equal(controlplanev1alpha1.CIDRClaimStatusStateBindingError))
	})
})
