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

package main

import (
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/go-logr/zapr"
	controlplanev1alpha1 "github.com/miscord-dev/toxfu/controlplane/api/v1alpha1"
	"github.com/miscord-dev/toxfu/pkg/alarm"
	"github.com/miscord-dev/toxfu/pkg/monitor"
	"github.com/miscord-dev/toxfu/toxfuengine"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdapiv1 "k8s.io/client-go/tools/clientcmd/api/v1"

	clientmiscordwinv1alpha1 "github.com/miscord-dev/toxfu/toxfud/api/v1alpha1"
	"github.com/miscord-dev/toxfu/toxfud/controllers"
	"github.com/miscord-dev/toxfu/toxfud/pkg/labels"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(clientmiscordwinv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	utilruntime.Must(controlplanev1alpha1.AddToScheme(scheme))

	utilruntime.Must(clientcmdapiv1.AddToScheme(scheme))
}

func main() {
	ctx := ctrl.SetupSignalHandler()

	flagSet := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flagSet.Usage = func() {
		fmt.Fprintf(flagSet.Output(), "Usage of %s:\n", os.Args[0])
		flagSet.PrintDefaults()
	}

	var metricsAddr string
	var probeAddr string
	configPath := "/etc/toxfu/toxfud.yaml"
	if c := os.Getenv("TOXFU_DAEMON_CONFIG"); c != "" {
		configPath = c
	}

	flagSet.StringVar(&metricsAddr, "metrics-bind-address", ":8090", "The address the metric endpoint binds to.")
	flagSet.StringVar(&probeAddr, "health-probe-bind-address", ":8091", "The address the probe endpoint binds to.")
	flagSet.StringVar(&configPath, "config", configPath, "Paths to a toxfu config.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flagSet)
	flagSet.Parse(os.Args[1:])

	logrLogger := zap.New(zap.UseFlagOptions(&opts))
	zapLogger := logrLogger.GetSink().(zapr.Underlier).GetUnderlying()

	ctrl.SetLogger(logrLogger)

	options := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   10443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         false,
		LeaderElectionID:       "ddc89635.client.miscord.win",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	}

	var config clientmiscordwinv1alpha1.CNIConfig

	options.AndFromOrDie(ctrl.ConfigFile().AtPath(configPath).OfKind(&config))
	options.LeaderElection = false

	config.LoadFromEnv(configPath)
	options.Namespace = config.ControlPlane.Namespace

	if config.Wireguard.PrivateKey == "" {
		privKey, err := wgtypes.GeneratePrivateKey()

		if err != nil {
			setupLog.Error(err, "failed to generate private key")
			os.Exit(1)
		}

		config.Wireguard.PrivateKey = privKey.String()
	}

	engine, err := toxfuengine.New(config.Wireguard.Name, config.Wireguard.VRF, uint32(config.Wireguard.Table), &toxfuengine.Config{
		PrivateKey:   config.Wireguard.PrivateKey,
		ListenPort:   config.Wireguard.ListenPort,
		STUNEndpoint: config.Wireguard.STUNEndpoint,
	}, zapLogger.Named("toxfu_core"))

	if err != nil {
		setupLog.Error(err, "failed to setup toxfu core")
		os.Exit(1)
	}
	defer func() {
		if config.Cleanup {
			engine.Close()
		}
	}()

	mon, err := monitor.New(zapLogger.Named("monitor"))

	if err != nil {
		setupLog.Error(err, "failed to set up monitor")
		os.Exit(1)
	}

	go func() {
		s := alarm.NewCombinator(alarm.NewTicker(30*time.Second), mon.Subscribe())
		defer s.Close()

		for {
			select {
			case <-s.C():
			case <-ctx.Done():
				return
			}

			engine.Trigger()
		}
	}()

	var restConfig *rest.Config
	if config.ControlPlane.APIEndpoint != "" {
		restConfig = &rest.Config{
			Host:        config.ControlPlane.APIEndpoint,
			BearerToken: config.ControlPlane.Token,
			TLSClientConfig: rest.TLSClientConfig{
				CAData: []byte(config.ControlPlane.RootCACert),
			},
		}
	} else if config.ControlPlane.KubeConfigRaw != nil {
		restConfig, err = clientcmd.BuildConfigFromKubeconfigGetter("", func() (*clientcmdapi.Config, error) {
			var cfg clientcmdapi.Config

			err := scheme.Convert(config.ControlPlane.KubeConfigRaw, &cfg, nil)

			if err != nil {
				return nil, err
			}

			return &cfg, nil
		})

		if err != nil {
			setupLog.Error(err, "setting rest config from kubeconfigraw for controller failed")
			os.Exit(1)
		}
	} else {
		restConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{
				ExplicitPath: config.ControlPlane.KubeConfig,
			},
			&clientcmd.ConfigOverrides{
				CurrentContext: config.ControlPlane.Context,
			},
		).ClientConfig()

		if err != nil {
			setupLog.Error(err, "setting rest config for controller failed")
			os.Exit(1)
		}
	}

	mgr, err := ctrl.NewManager(restConfig, options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.CIDRClaimerReconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		ControlPlaneNamespace: config.ControlPlane.Namespace,
		ClusterName:           config.ClusterName,
		NodeName:              config.NodeName,
		TemplateNames:         config.ControlPlane.AddressClaimTemplates,
		ClaimNameGenerator: func(templateName string) string {
			name := fmt.Sprintf("%s-%s-%s", config.ClusterName, config.NodeName, templateName)

			if len(name) < 53 {
				return name
			}

			hash := sha1.Sum([]byte(name))

			return name[:53-9] + "-" + hex.EncodeToString(hash[:])[:8]
		},
		Labels: func(templateName string) map[string]string {
			return labels.NodeTypeForNode(config.ClusterName, config.NodeName)
		},
	}).SetupWithManager(mgr, "NodeAddressSync"); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CIDRClaimer")
		os.Exit(1)
	}
	if err = (&controllers.PeerNodeSyncReconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		ControlPlaneNamespace: config.ControlPlane.Namespace,
		ClusterName:           config.ClusterName,
		NodeName:              config.NodeName,
		Engine:                engine,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PeerNodeSync")
		os.Exit(1)
	}
	if err = (&controllers.PeersSyncReconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		ControlPlaneNamespace: config.ControlPlane.Namespace,
		ClusterName:           config.ClusterName,
		NodeName:              config.NodeName,
		Engine:                engine,

		PrivateKey:   config.Wireguard.PrivateKey,
		ListenPort:   config.Wireguard.ListenPort,
		STUNEndpoint: config.Wireguard.STUNEndpoint,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PeersSync")
		os.Exit(1)
	}

	//+kubebuilder:scaffold:builder

	// Setup controllers for CNI
	SetupCNId(ctx, mgr, config)

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")

	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	setupLog.Info("Stopping")
}
