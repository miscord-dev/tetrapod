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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PeerNodeSpec defines the desired state of PeerNode
type PeerNodeSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// PublicKey is a Wireguard public key
	PublicKey string `json:"publicKey,omitempty"`

	// PublicDiscoKey is a public key for Disco
	PublicDiscoKey string `json:"publicDiscoKey,omitempty"`

	// Attributes is a metadata of the node
	Attributes Attributes `json:"attributes,omitempty"`

	// StaticRoutes are the CIDRs to be routed
	StaticRoutes []string `json:"staticRoutes,omitempty"`

	// CIDRClaims are the requests of ip addresses
	// +patchMergeKey=name
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=name
	CIDRClaims []PeerNodeSpecCIDRClaim `json:"addressClaims"`
}

type Attributes struct {
	// HostName is a host name of the node
	HostName string `json:"hostName"`

	// OS is the OS name
	OS string `json:"os,omitempty"`

	// Arch is the CPU architecture
	Arch string `json:"arch,omitempty"`
}

type PeerNodeSpecCIDRClaim struct {
	// Name is the identifier of claim
	Name string `json:"name"`

	// Selector is a labal selector of IPAddressRange
	Selector metav1.LabelSelector `json:"selector"`

	// SizeBit is log2(the number of requested addresses)
	// Must be 2^N (N>=0)
	// +kubebuilder:default=0
	SizeBit int `json:"size"`
}

type PeerNodeStatusState string

const (
	// PeerNodeStatusStateUnknown represents the unknown state
	PeerNodeStatusStateUnknown PeerNodeStatusState = ""

	// PeerNodeStatusStateReady represents the ready state
	PeerNodeStatusStateReady PeerNodeStatusState = "ready"

	// PeerNodeStatusStateUpdating represents the updating state
	PeerNodeStatusStateUpdating PeerNodeStatusState = "updating"

	// PeerNodeStatusStateBindingError represents the updating state
	PeerNodeStatusStateBindingError PeerNodeStatusState = "bindingError"
)

type PeerNodeStatusCIDRClaim struct {
	// Name is the identifier of claim
	Name string `json:"name"`

	// Ready represents the addresses for the last claim are allocated
	Ready bool `json:"ready"`

	// Message is the error message
	Message string `json:"message,omitempty"`

	// CIDR represents the block of asiggned addresses like 192.168.1.0/24, [fe80::]/32
	CIDR string `json:"cidr,omitempty"`

	// Size is log2(the number of requested addresses)
	SizeBit int `json:"size,omitempty"`
}

// PeerNodeStatus defines the observed state of PeerNode
type PeerNodeStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ObservedGeneration is the observed generation
	ObservedGeneration int64 `json:"observedGeneration"`

	// State represents the current state
	State PeerNodeStatusState `json:"state"`

	// Message is the error message
	Message string `json:"message,omitempty"`

	// CIDRClaims are assigned addresses for this PeerNode
	// +patchMergeKey=name
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=name
	CIDRClaims []PeerNodeStatusCIDRClaim `json:"cidrClaims,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PeerNode is the Schema for the peernodes API
type PeerNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PeerNodeSpec   `json:"spec,omitempty"`
	Status PeerNodeStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PeerNodeList contains a list of PeerNode
type PeerNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PeerNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PeerNode{}, &PeerNodeList{})
}
