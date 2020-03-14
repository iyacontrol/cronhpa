// NOTE: Boilerplate only.  Ignore this file.

// Package v1beta1 contains API Schema definitions for the confighpas v1beta1 API group
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen=package,register
// +k8s:conversion-gen=github.com/iyacontrol/cronhpa/pkg/apis/cronhpa
// +k8s:defaulter-gen=TypeMeta
// +groupName=cronhpa.shareit.me
package v1beta1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// SchemeGroupVersion is group version used to register these objects
	SchemeGroupVersion = schema.GroupVersion{Group: "cronhpa.shareit.me", Version: "v1beta1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}
)
