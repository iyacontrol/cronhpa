package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CrossVersionObjectReference contains enough information to let you identify the referred resource.
type CrossVersionObjectReference struct {
	// Kind of the referent; More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds"
	Kind string `json:"kind"`
	// Name of the referent; More info: http://kubernetes.io/docs/user-guide/identifiers#names
	Name string `json:"name"`
	// API version of the referent
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// CronHPA represents a set of crontabs to set target's replicas.
type CronHPA struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired identities of pods in this cronhpa.
	Spec CronHPASpec `json:"spec,omitempty"`

	// Status is the current status of pods in this CronHPA. This data
	// may be out of date by some window of time.
	Status CronHPAStatus `json:"status,omitempty"`
}

// A CronHPASpec is the specification of a CronHPA.
type CronHPASpec struct {
	// scaleTargetRef points to the target resource to scale
	ScaleTargetRef CrossVersionObjectReference `json:"scaleTargetRef" protobuf:"bytes,1,opt,name=scaleTargetRef"`

	Crons []Cron `json:"crons" protobuf:"bytes,2,opt,name=crons"`
}

type Cron struct {
	// The schedule in Cron format, see https://en.wikipedia.org/wiki/Cron.
	Schedule string `json:"schedule" protobuf:"bytes,1,opt,name=schedule"`

	TargetReplicas int32 `json:"targetReplicas" protobuf:"varint,2,opt,name=targetReplicas"`
}


// CronHPAStatus represents the current state of a CronHPA.
type CronHPAStatus struct {
	// Information when was the last time the schedule was successfully scheduled.
	// +optional
	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty" protobuf:"bytes,2,opt,name=lastScheduleTime"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CronHPAList is a collection of CronHPA.
type CronHPAList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CronHPA `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CronHPA{}, &CronHPAList{})
}
