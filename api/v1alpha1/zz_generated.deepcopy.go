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
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Encoding) DeepCopyInto(out *Encoding) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Encoding.
func (in *Encoding) DeepCopy() *Encoding {
	if in == nil {
		return nil
	}
	out := new(Encoding)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SinkSpec) DeepCopyInto(out *SinkSpec) {
	*out = *in
	if in.Inputs != nil {
		in, out := &in.Inputs, &out.Inputs
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Encoding != nil {
		in, out := &in.Encoding, &out.Encoding
		*out = new(Encoding)
		**out = **in
	}
	if in.Rate != nil {
		in, out := &in.Rate, &out.Rate
		*out = new(int32)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SinkSpec.
func (in *SinkSpec) DeepCopy() *SinkSpec {
	if in == nil {
		return nil
	}
	out := new(SinkSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SourceSpec) DeepCopyInto(out *SourceSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SourceSpec.
func (in *SourceSpec) DeepCopy() *SourceSpec {
	if in == nil {
		return nil
	}
	out := new(SourceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Vector) DeepCopyInto(out *Vector) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Vector.
func (in *Vector) DeepCopy() *Vector {
	if in == nil {
		return nil
	}
	out := new(Vector)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Vector) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VectorAgent) DeepCopyInto(out *VectorAgent) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VectorAgent.
func (in *VectorAgent) DeepCopy() *VectorAgent {
	if in == nil {
		return nil
	}
	out := new(VectorAgent)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VectorAggregator) DeepCopyInto(out *VectorAggregator) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VectorAggregator.
func (in *VectorAggregator) DeepCopy() *VectorAggregator {
	if in == nil {
		return nil
	}
	out := new(VectorAggregator)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VectorList) DeepCopyInto(out *VectorList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Vector, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VectorList.
func (in *VectorList) DeepCopy() *VectorList {
	if in == nil {
		return nil
	}
	out := new(VectorList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *VectorList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VectorPipeline) DeepCopyInto(out *VectorPipeline) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VectorPipeline.
func (in *VectorPipeline) DeepCopy() *VectorPipeline {
	if in == nil {
		return nil
	}
	out := new(VectorPipeline)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *VectorPipeline) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VectorPipelineList) DeepCopyInto(out *VectorPipelineList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]VectorPipeline, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VectorPipelineList.
func (in *VectorPipelineList) DeepCopy() *VectorPipelineList {
	if in == nil {
		return nil
	}
	out := new(VectorPipelineList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *VectorPipelineList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VectorPipelineSpec) DeepCopyInto(out *VectorPipelineSpec) {
	*out = *in
	if in.Source != nil {
		in, out := &in.Source, &out.Source
		*out = make(map[string]SourceSpec, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Sink != nil {
		in, out := &in.Sink, &out.Sink
		*out = make(map[string]SinkSpec, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VectorPipelineSpec.
func (in *VectorPipelineSpec) DeepCopy() *VectorPipelineSpec {
	if in == nil {
		return nil
	}
	out := new(VectorPipelineSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VectorPipelineStatus) DeepCopyInto(out *VectorPipelineStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VectorPipelineStatus.
func (in *VectorPipelineStatus) DeepCopy() *VectorPipelineStatus {
	if in == nil {
		return nil
	}
	out := new(VectorPipelineStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VectorSpec) DeepCopyInto(out *VectorSpec) {
	*out = *in
	if in.Agent != nil {
		in, out := &in.Agent, &out.Agent
		*out = new(VectorAgent)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VectorSpec.
func (in *VectorSpec) DeepCopy() *VectorSpec {
	if in == nil {
		return nil
	}
	out := new(VectorSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VectorStatus) DeepCopyInto(out *VectorStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VectorStatus.
func (in *VectorStatus) DeepCopy() *VectorStatus {
	if in == nil {
		return nil
	}
	out := new(VectorStatus)
	in.DeepCopyInto(out)
	return out
}
