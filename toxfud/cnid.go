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
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func SetupCNId(ctx context.Context, mgr manager.Manager, config clientmiscordwinv1alpha1.CNIConfig) {
	if len(config.CNID.AddressClaimTemplates) == 0 {
		return
	}

	if err := (&controllers.CIDRClaimerReconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		ControlPlaneNamespace: config.ControlPlane.Namespace,
		ClusterName:           config.ClusterName,
		NodeName:              config.NodeName,
		TemplateNames:         config.CNID.AddressClaimTemplates,
		ClaimNameGenerator: func(templateName string) string {
			name := fmt.Sprintf("%s-%s-pod-%s", config.ClusterName, config.NodeName, templateName)

			if len(name) < 53 {
				return name
			}

			hash := sha1.Sum([]byte(name))

			return name[:53-9] + "-" + hex.EncodeToString(hash[:])[:8]
		},
		Labels: func(templateName string) map[string]string {
			return labels.PodCIDRTypeForNode(config.ClusterName, config.NodeName, templateName)
		},
	}).SetupWithManager(mgr, "PodCIDRSync"); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CIDRClaimer")
		os.Exit(1)
	}

	var localCache cache.Cache
	if config.CNID.Extra {
		restConfig, err := loadRestConfigFromKubeConfig(scheme, &config.CNID.KubeConfig)

		if err != nil {
			setupLog.Error(err, "setting rest config for controller failed")
			os.Exit(1)
		}

		localCluster, err := cluster.New(restConfig)

		if err != nil {
			setupLog.Error(err, "setting up local cluster failed")
			os.Exit(1)
		}
		localCache = localCluster.GetCache()

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
		Cache:                    mgr.GetCache(),
		LocalCache:               localCache,
		ControlPlaneNamespace:    config.ControlPlane.Namespace,
		ClusterName:              config.ClusterName,
		NodeName:                 config.NodeName,
		PodAddressClaimTemplates: config.ControlPlane.AddressClaimTemplates,
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
