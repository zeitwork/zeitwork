/*
Copyright 2025.

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

// AppRevisionSpec defines the desired state of AppRevision
type AppRevisionSpec struct {
	CommitSHA string `json:"commitSHA"`
}

// AppRevisionStatus defines the observed state of AppRevision
type AppRevisionStatus struct {
	ImageBuilt *string `json:"imageBuilt,omitempty"`
	Ready      bool    `json:"ready"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AppRevision is the Schema for the apprevisions API
type AppRevision struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppRevisionSpec   `json:"spec,omitempty"`
	Status AppRevisionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AppRevisionList contains a list of AppRevision
type AppRevisionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppRevision `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AppRevision{}, &AppRevisionList{})
}
