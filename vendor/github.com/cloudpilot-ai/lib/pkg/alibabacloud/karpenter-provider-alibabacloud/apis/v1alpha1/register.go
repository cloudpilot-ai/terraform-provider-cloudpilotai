package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/cloudpilot-ai/lib/pkg/alibabacloud/karpenter-provider-alibabacloud/apis"
)

var (
	SchemeGroupVersion = schema.GroupVersion{Group: apis.Group, Version: "v1alpha1"}
	SchemeBuilder      = runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
		scheme.AddKnownTypes(SchemeGroupVersion,
			&ECSNodeClass{},
			&ECSNodeClassList{},
		)
		return nil
	})
)
