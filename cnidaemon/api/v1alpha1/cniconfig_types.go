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
	"os"

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

	AddressClaimTemplate string `json:"addressClaimTemplate"`
}

func loadFromEnv(v *string, key string) {
	value := os.Getenv(key)

	if value != "" {
		return
	}

	*v = value
}

func (cp *ControlPlane) LoadFromEnv() {
	loadFromEnv(&cp.APIEndpoint, "TOXFU_CONTROLPLANE_APIENDPOINT")
	loadFromEnv(&cp.RootCACert, "TOXFU_CONTROLPLANE_ROOT_CA_CERT")
	loadFromEnv(&cp.Token, "TOXFU_CONTROLPLANE_TOKEN")
	loadFromEnv(&cp.Namespace, "TOXFU_CONTROLPLANE_NAMESPACE")
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
}

func (cc *CNIConfig) LoadFromEnv() {
	loadFromEnv(&cc.ClusterName, "TOXFU_CLUSTER_NAME")
	loadFromEnv(&cc.NodeName, "TOXFU_NODE_NAME")
	loadFromEnv(&cc.NetworkNamespace, "TOXFU_NETNS")

	cc.ControlPlane.LoadFromEnv()
}
