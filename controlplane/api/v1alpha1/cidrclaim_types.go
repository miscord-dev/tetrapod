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

// CIDRClaimSpec defines the desired state of CIDRClaim
type CIDRClaimSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Selector is a labal selector of CIDRBlock
	Selector metav1.LabelSelector `json:"selector"`

	// SizeBit is log2(the number of requested addresses)
	// +kubebuilder:default=0
	SizeBit int `json:"sizeBit"`
}

type CIDRClaimStatusState string

const (
	// CIDRClaimStatusStateUnknown represents the unknown state
	CIDRClaimStatusStateUnknown CIDRClaimStatusState = ""

	// CIDRClaimStatusStateReady represents the ready state
	CIDRClaimStatusStateReady CIDRClaimStatusState = "ready"

	// CIDRClaimStatusStateBindingError represents the updating state
	CIDRClaimStatusStateBindingError CIDRClaimStatusState = "bindingError"
)

// CIDRClaimStatus defines the observed state of CIDRClaim
type CIDRClaimStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ObservedGeneration is the observed generation
	ObservedGeneration int64 `json:"observedGeneration"`

	// State represents the current state
	State CIDRClaimStatusState `json:"state"`

	// Message is the error message
	Message string `json:"message,omitempty"`

	// Name of the CIDRBlock
	CIDRBlockName string `json:"name,omitempty"`

	// CIDR represents the block of asiggned addresses like 192.168.1.0/24, [fe80::]/32
	CIDR string `json:"cidr,omitempty"`

	// SizeBit is log2(the number of requested addresses)
	SizeBit int `json:"sizeBit,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CIDRClaim is the Schema for the cidrclaims API
type CIDRClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CIDRClaimSpec   `json:"spec,omitempty"`
	Status CIDRClaimStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CIDRClaimList contains a list of CIDRClaim
type CIDRClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CIDRClaim `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CIDRClaim{}, &CIDRClaimList{})
}
