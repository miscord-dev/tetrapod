package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"

	clientmiscordwinv1alpha1 "github.com/miscord-dev/toxfu/toxfud/api/v1alpha1"
	"github.com/miscord-dev/toxfu/toxfud/controllers"
	"github.com/miscord-dev/toxfu/toxfud/pkg/cniserver"
	"github.com/miscord-dev/toxfu/toxfud/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func SetupCNId(ctx context.Context, mgr manager.Manager, config clientmiscordwinv1alpha1.CNIConfig) {
	if config.CNID.AddressClaimTemplate == "" {
		return
	}

	if err := (&controllers.CIDRClaimerReconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		ControlPlaneNamespace: config.ControlPlane.Namespace,
		ClusterName:           config.ClusterName,
		NodeName:              config.NodeName,
		TemplateName:          config.CNID.AddressClaimTemplate,
		ClaimNameGenerator: func() string {
			name := fmt.Sprintf("%s-%s-pod", config.ClusterName, config.NodeName)

			if len(name) < 53 {
				return name
			}

			hash := sha1.Sum([]byte(name))

			return name[:53-9] + "-" + hex.EncodeToString(hash[:])[:8]
		},
		Labels: func() map[string]string {
			return labels.NodeTypeForPodCIDR(config.ClusterName, config.NodeName)
		},
	}).SetupWithManager(mgr, "PodCIDRSync"); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CIDRClaimer")
		os.Exit(1)
	}

	if config.CNID.Extra {
		localCluster, err := cluster.New(ctrl.GetConfigOrDie())

		if err != nil {
			setupLog.Error(err, "setting up local cluster failed")
			os.Exit(1)
		}

		if err := (&controllers.ExtraPodCIDRSyncReconciler{
			Client:                mgr.GetClient(),
			Scheme:                mgr.GetScheme(),
			ControlPlaneNamespace: config.ControlPlane.Namespace,
			ClusterName:           config.ClusterName,
			NodeName:              config.NodeName,
			Local:                 localCluster,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ExtraPodCIDRSync")
			os.Exit(1)
		}
	}

	server, err := cniserver.NewServer(config.CNID.SocketPath, cniserver.Options{
		Cache:                 mgr.GetCache(),
		ControlPlaneNamespace: config.ControlPlane.Namespace,
		ClusterName:           config.ClusterName,
		NodeName:              config.NodeName,
	})
	if err != nil {
		setupLog.Error(err, "unable to start cni server")
		os.Exit(1)
	}

	go server.Run()
	go func() {
		<-ctx.Done()
		server.Shutdown()
	}()
}
