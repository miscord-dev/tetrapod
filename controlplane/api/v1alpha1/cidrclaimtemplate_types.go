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

// CIDRClaimTemplateSpec defines the desired state of CIDRClaimTemplate
type CIDRClaimTemplateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Selector is a labal selector of CIDRBlock
	Selector metav1.LabelSelector `json:"selector"`

	// SizeBit is log2(the number of requested addresses)
	// +kubebuilder:default=0
	SizeBit int `json:"sizeBit"`
}

// CIDRClaimTemplateStatus defines the observed state of CIDRClaimTemplate
type CIDRClaimTemplateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CIDRClaimTemplate is the Schema for the cidrclaimtemplates API
type CIDRClaimTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CIDRClaimTemplateSpec   `json:"spec,omitempty"`
	Status CIDRClaimTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CIDRClaimTemplateList contains a list of CIDRClaimTemplate
type CIDRClaimTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CIDRClaimTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CIDRClaimTemplate{}, &CIDRClaimTemplateList{})
}
