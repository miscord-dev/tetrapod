//go:build !ignore_autogenerated
// +build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd/api/v1"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CNIConfig) DeepCopyInto(out *CNIConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ControllerManagerConfigurationSpec.DeepCopyInto(&out.ControllerManagerConfigurationSpec)
	in.ControlPlane.DeepCopyInto(&out.ControlPlane)
	out.Wireguard = in.Wireguard
	if in.StaticAdvertisedRoutes != nil {
		in, out := &in.StaticAdvertisedRoutes, &out.StaticAdvertisedRoutes
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	in.CNID.DeepCopyInto(&out.CNID)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CNIConfig.
func (in *CNIConfig) DeepCopy() *CNIConfig {
	if in == nil {
		return nil
	}
	out := new(CNIConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CNIConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CNIDConfig) DeepCopyInto(out *CNIDConfig) {
	*out = *in
	if in.AddressClaimTemplates != nil {
		in, out := &in.AddressClaimTemplates, &out.AddressClaimTemplates
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	in.KubeConfig.DeepCopyInto(&out.KubeConfig)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CNIDConfig.
func (in *CNIDConfig) DeepCopy() *CNIDConfig {
	if in == nil {
		return nil
	}
	out := new(CNIDConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ControlPlane) DeepCopyInto(out *ControlPlane) {
	*out = *in
	in.KubeConfig.DeepCopyInto(&out.KubeConfig)
	if in.AddressClaimTemplates != nil {
		in, out := &in.AddressClaimTemplates, &out.AddressClaimTemplates
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ControlPlane.
func (in *ControlPlane) DeepCopy() *ControlPlane {
	if in == nil {
		return nil
	}
	out := new(ControlPlane)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KubeConfig) DeepCopyInto(out *KubeConfig) {
	*out = *in
	if in.Inline != nil {
		in, out := &in.Inline, &out.Inline
		*out = new(v1.Config)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KubeConfig.
func (in *KubeConfig) DeepCopy() *KubeConfig {
	if in == nil {
		return nil
	}
	out := new(KubeConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Wireguard) DeepCopyInto(out *Wireguard) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Wireguard.
func (in *Wireguard) DeepCopy() *Wireguard {
	if in == nil {
		return nil
	}
	out := new(Wireguard)
	in.DeepCopyInto(out)
	return out
}
