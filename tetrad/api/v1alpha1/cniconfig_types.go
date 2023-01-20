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

package v1alpha1

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/miscord-dev/tetrapod/tetrad/pkg/cniserver"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientcmdapiv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	configv1alpha1 "sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

func loadFromEnv(v *string, key string) {
	value := os.Getenv(key)

	if value == "" {
		return
	}

	*v = value
}

func loadFromEnvArray(v *[]string, key string) {
	value := os.Getenv(key)

	if value == "" {
		return
	}

	for _, elm := range strings.Split(value, ",") {
		*v = append(*v, strings.TrimSpace(elm))
	}
}

type KubeConfig struct {
	File    string                 `json:"file"`
	Inline  *clientcmdapiv1.Config `json:"inline"`
	Context string                 `json:"context"`
}

func (kc *KubeConfig) Load(configPath string) {
	if kc.File != "" && kc.File != "in-cluster" && !filepath.IsAbs(kc.File) {
		kc.File = filepath.Join(filepath.Dir(configPath), kc.File)
	}
}

type ControlPlane struct {
	APIEndpoint string     `json:"apiEndpoint"`
	RootCACert  string     `json:"rootCACert"`
	Token       string     `json:"token"`
	Namespace   string     `json:"namespace"`
	KubeConfig  KubeConfig `json:"kubeconfig"`

	AddressClaimTemplates []string `json:"addressClaimTemplates"`
}

func (cp *ControlPlane) Load(configPath string) {
	loadFromEnv(&cp.APIEndpoint, "TETRAPOD_CONTROLPLANE_APIENDPOINT")
	loadFromEnv(&cp.RootCACert, "TETRAPOD_CONTROLPLANE_ROOT_CA_CERT")
	loadFromEnv(&cp.Token, "TETRAPOD_CONTROLPLANE_TOKEN")
	loadFromEnv(&cp.Namespace, "TETRAPOD_CONTROLPLANE_NAMESPACE")
	loadFromEnv(&cp.KubeConfig.File, "TETRAPOD_CONTROLPLANE_KUBECONFIG")
	loadFromEnv(&cp.KubeConfig.Context, "TETRAPOD_CONTROLPLANE_CONTEXT")
	loadFromEnvArray(&cp.AddressClaimTemplates, "TETRAPOD_CONTROLPLANE_TEMPLATES")

	if cp.Namespace == "" {
		cp.Namespace = "default"
	}

	cp.KubeConfig.Load(configPath)
}

type Wireguard struct {
	PrivateKey   string `json:"privateKey"`
	ListenPort   int    `json:"listenPort"`
	STUNEndpoint string `json:"stunEndpoint"`
	Name         string `json:"name"`
	VRF          string `json:"vrf"`
	Table        int    `json:"table"`
}

func (wg *Wireguard) Load() {
	loadFromEnv(&wg.PrivateKey, "TETRAPOD_WG_PRIVATE_KEY")
	loadFromEnv(&wg.STUNEndpoint, "TETRAPOD_WG_STUN_ENDPOINT")

	var listenPort string
	loadFromEnv(&listenPort, "TETRAPOD_WG_LISTEN_PORT")
	if listenPort != "" {
		var err error
		wg.ListenPort, err = strconv.Atoi(listenPort)

		if err != nil {
			panic(fmt.Sprintf("%s cannot be parsed into int", listenPort))
		}
	}

	if wg.ListenPort == 0 {
		wg.ListenPort = 54321
	}
	if wg.PrivateKey == "" {
		wg.loadPrivateKeyFromDisk()
	}

	if wg.STUNEndpoint == "" {
		wg.STUNEndpoint = "stun.l.google.com:19302"
	}

	if wg.Name == "" {
		wg.Name = "tetrapod0"
	}
	if wg.VRF == "" {
		wg.VRF = "tetrapod-vrf"
	} else if wg.VRF == "-" {
		wg.VRF = ""
	}
	if wg.Table == 0 {
		wg.Table = 1351
	}
}

func (wg *Wireguard) loadPrivateKeyFromDisk() {
	dir := "/etc/tetrapod/keys"
	keyFile := filepath.Join(dir, "private_key")

	if b, _ := os.ReadFile(keyFile); len(b) != 0 {
		wg.PrivateKey = strings.TrimSpace(string(b))
	}

	key, err := wgtypes.GeneratePrivateKey()

	if err != nil {
		panic(err)
	}

	wg.PrivateKey = key.String()

	os.MkdirAll(dir, 0700)
	os.WriteFile(keyFile, []byte(key.String()), 0700)
}

type CNIDConfig struct {
	AddressClaimTemplates []string   `json:"addressClaimTemplates"`
	Extra                 bool       `json:"extra"`
	SocketPath            string     `json:"socketPath"`
	KubeConfig            KubeConfig `json:"kubeconfig"`
}

func (c *CNIDConfig) Load(configPath string) {
	if c.SocketPath == "" {
		c.SocketPath = cniserver.DefaultSocketPath
	}
	if c.KubeConfig.File == "" && c.KubeConfig.Inline == nil {
		c.KubeConfig.File = "in-cluster"
	}

	c.KubeConfig.Load(configPath)
}

//+kubebuilder:object:root=true

// CNIConfig is the Schema for the cniconfigs API
type CNIConfig struct {
	metav1.TypeMeta                                   `json:",inline"`
	configv1alpha1.ControllerManagerConfigurationSpec `json:",inline"`
	ClusterName                                       string       `json:"clusterName"`
	NodeName                                          string       `json:"nodeName"`
	ControlPlane                                      ControlPlane `json:"controlPlane"`
	NetworkNamespace                                  string       `json:"networkNamespace"`
	Wireguard                                         Wireguard    `json:"wireguard"`
	Cleanup                                           bool         `json:"cleanup"`
	StaticAdvertisedRoutes                            []string     `json:"staticAdvertisedRoutes"`
	CNID                                              CNIDConfig   `json:"cnid"`
}

func (cc *CNIConfig) Load(configPath string) error {
	loadFromEnv(&cc.ClusterName, "TETRAPOD_CLUSTER_NAME")
	loadFromEnv(&cc.NodeName, "TETRAPOD_NODE_NAME")
	loadFromEnv(&cc.NetworkNamespace, "TETRAPOD_NETNS")

	cc.ControlPlane.Load(configPath)
	cc.Wireguard.Load()
	cc.CNID.Load(configPath)

	for _, route := range cc.StaticAdvertisedRoutes {
		_, _, err := net.ParseCIDR(route)

		if err != nil {
			return fmt.Errorf("failed to parse static advertised route %s: %w", route, err)
		}
	}

	return nil
}
