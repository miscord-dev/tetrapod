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
	"os"
	"strconv"

	"github.com/miscord-dev/toxfu/toxfud/pkg/cniserver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	configv1alpha1 "sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ControlPlane struct {
	APIEndpoint string `json:"apiEndpoint"`
	RootCACert  string `json:"rootCACert"`
	Token       string `json:"-"`
	Namespace   string `json:"namespace"`
	KubeConfig  string `json:"kubeconfig"`
	Context     string `json:"context"`

	AddressClaimTemplates []string `json:"addressClaimTemplates"`
}

func loadFromEnv(v *string, key string) {
	value := os.Getenv(key)

	if value == "" {
		return
	}

	*v = value
}

func (cp *ControlPlane) LoadFromEnv() {
	loadFromEnv(&cp.APIEndpoint, "TOXFU_CONTROLPLANE_APIENDPOINT")
	loadFromEnv(&cp.RootCACert, "TOXFU_CONTROLPLANE_ROOT_CA_CERT")
	loadFromEnv(&cp.Token, "TOXFU_CONTROLPLANE_TOKEN")
	loadFromEnv(&cp.Namespace, "TOXFU_CONTROLPLANE_NAMESPACE")
	loadFromEnv(&cp.KubeConfig, "TOXFU_CONTROLPLANE_KUBECONFIG")
	loadFromEnv(&cp.Context, "TOXFU_CONTROLPLANE_CONTEXT")
}

type Wireguard struct {
	PrivateKey   string `json:"privateKey"`
	ListenPort   int    `json:"listenPort"`
	STUNEndpoint string `json:"stunEndpoint"`
	Name         string `json:"name"`
	VRF          string `json:"vrf"`
	Table        int    `json:"table"`
}

func (wg *Wireguard) LoadFromEnv() {
	loadFromEnv(&wg.PrivateKey, "TOXFU_WG_PRIVATE_KEY")
	loadFromEnv(&wg.STUNEndpoint, "TOXFU_WG_STUN_ENDPOINT")

	var listenPort string
	loadFromEnv(&listenPort, "TOXFU_WG_LISTEN_PORT")
	if listenPort != "" {
		var err error
		wg.ListenPort, err = strconv.Atoi(listenPort)

		if err != nil {
			panic(fmt.Sprintf("%s cannot be parsed into int", listenPort))
		}
	}

	if wg.Name == "" {
		wg.Name = "toxfu0"
	}
	if wg.VRF == "" {
		wg.VRF = "toxfu-vrf"
	}
	if wg.Table == 0 {
		wg.Table = 1351
	}
}

type CNIDConfig struct {
	AddressClaimTemplates []string `json:"addressClaimTemplates"`
	Extra                 bool     `json:"extra"`
	SocketPath            string   `json:"socketPath"`
}

func (c *CNIDConfig) LoadFromEnv() {
	if c.SocketPath == "" {
		c.SocketPath = cniserver.DefaultSocketPath
	}
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
	CNID                                              CNIDConfig   `json:"cnid"`
}

func (cc *CNIConfig) LoadFromEnv() {
	loadFromEnv(&cc.ClusterName, "TOXFU_CLUSTER_NAME")
	loadFromEnv(&cc.NodeName, "TOXFU_NODE_NAME")
	loadFromEnv(&cc.NetworkNamespace, "TOXFU_NETNS")

	cc.ControlPlane.LoadFromEnv()
	cc.Wireguard.LoadFromEnv()
	cc.CNID.LoadFromEnv()
}
