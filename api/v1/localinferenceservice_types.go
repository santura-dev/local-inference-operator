// Copyright 2025 Sandra Poturalska
// SPDX-License-Identifier: MIT

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// LocalInferenceServiceSpec defines the desired state of LocalInferenceService
type LocalInferenceServiceSpec struct {
	// Runtime specifies the inference runtime (e.g., "vllm" or "sglang")
	// +kubebuilder:validation:Enum=vllm;sglang
	Runtime string `json:"runtime"`

	// Model defines the model configuration
	Model ModelSpec `json:"model"`

	// Settings defines advanced inference settings
	// +optional
	Settings SettingsSpec `json:"settings,omitempty"`

	// Scaling defines replica and autoscaling settings
	// +optional
	Scaling ScalingSpec `json:"scaling,omitempty"`
}

// ModelSpec defines the model to use for inference
type ModelSpec struct {
	// URI is the HuggingFace model URI (e.g., hf://facebook/opt-125m)
	URI string `json:"uri"`

	// Name is a human-readable name for the model
	// +optional
	Name string `json:"name,omitempty"`

	// Version specifies the model version
	// +optional
	Version string `json:"version,omitempty"`
}

// SettingsSpec defines advanced settings for inference
type SettingsSpec struct {
	// BatchSize specifies the batch size for inference
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	BatchSize int32 `json:"batchSize,omitempty"`

	// Precision specifies the numerical precision (e.g., "fp16", "fp32")
	// +kubebuilder:default="fp16"
	// +kubebuilder:validation:Enum=fp16;fp32;int8
	Precision string `json:"precision,omitempty"`

	// MaxTokens specifies the maximum number of tokens to generate
	// +kubebuilder:default=100
	// +kubebuilder:validation:Minimum=1
	MaxTokens int32 `json:"maxTokens,omitempty"`

	// GPUMemory specifies the GPU memory limit (e.g., "8Gi")
	// +optional
	GPUMemory string `json:"gpuMemory,omitempty"`
}

// ScalingSpec defines scaling configuration
type ScalingSpec struct {
	// Replicas specifies the number of pod replicas
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	Replicas int32 `json:"replicas,omitempty"`

	// AutoScale enables horizontal pod autoscaling
	// +optional
	AutoScale bool `json:"autoScale,omitempty"`

	// MinReplicas specifies the minimum number of replicas for autoscaling
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	MinReplicas int32 `json:"minReplicas,omitempty"`

	// MaxReplicas specifies the maximum number of replicas for autoscaling
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	MaxReplicas int32 `json:"maxReplicas,omitempty"`

	// TargetCPU specifies the target CPU utilization percentage for autoscaling
	// +kubebuilder:default=70
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	TargetCPU int32 `json:"targetCPU,omitempty"`
}

// LocalInferenceServiceStatus defines the observed state of LocalInferenceService.
type LocalInferenceServiceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the LocalInferenceService resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Phase represents the current phase of the LocalInferenceService
	// +optional
	Phase string `json:"phase,omitempty"`

	// DeploymentName is the name of the created Deployment
	// +optional
	DeploymentName string `json:"deploymentName,omitempty"`

	// ServiceName is the name of the created Service
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// ReadyReplicas indicates how many replicas are ready
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// LocalInferenceService is the Schema for the localinferenceservices API
type LocalInferenceService struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of LocalInferenceService
	// +required
	Spec LocalInferenceServiceSpec `json:"spec"`

	// status defines the observed state of LocalInferenceService
	// +optional
	Status LocalInferenceServiceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// LocalInferenceServiceList contains a list of LocalInferenceService
type LocalInferenceServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []LocalInferenceService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LocalInferenceService{}, &LocalInferenceServiceList{})
}
