package main

import (
	"context"
	"fmt"

	controlplanev1alpha1 "github.com/miscord-dev/toxfu/controlplane/api/v1alpha1"
	"github.com/miscord-dev/toxfu/toxfucni/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(controlplanev1alpha1.AddToScheme(scheme))
}

func initRESTConfig(cfg *config.Config) *rest.Config {
	tlsClientConfig := rest.TLSClientConfig{}
	if _, err := certutil.NewPoolFromBytes([]byte(cfg.ControlPlane.RootCACert)); err != nil {
		klog.Errorf("Expected to load root CA config from %s, but got err: %v", cfg.ControlPlane.RootCACert, err)
	} else {
		tlsClientConfig.CAData = []byte(cfg.ControlPlane.RootCACert)
	}

	restConfig := rest.Config{
		Host:            cfg.ControlPlane.APIEndpoint,
		BearerToken:     cfg.ControlPlane.Token,
		TLSClientConfig: tlsClientConfig,
	}

	return &restConfig
}

func main() {
	cfg, err := config.New()

	if err != nil {
		panic(err)
	}

	restConfig := initRESTConfig(cfg)

	cache, err := cache.New(restConfig, cache.Options{
		Scheme: scheme,
	})

	if err != nil {
		panic(err)
	}
	go func() {
		if err := cache.Start(context.Background()); err != nil {
			panic(err)
		}
	}()

	client, err := cluster.DefaultNewClient(cache, restConfig, client.Options{
		Scheme: scheme,
	})

	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	var namespaces corev1.NamespaceList
	if err := client.List(ctx, &namespaces); err != nil {
		panic(err)
	}

	for _, ns := range namespaces.Items {
		fmt.Println(ns.Name)
	}
}
