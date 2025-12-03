package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/cloudpilot-ai/lib/pkg/alibabacloud/karpenter/apis"
)

var (
	SchemeGroupVersion = schema.GroupVersion{Group: apis.Group, Version: "v1"}
	SchemeBuilder      = runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
		scheme.AddKnownTypes(SchemeGroupVersion,
			&NodePool{},
			&NodePoolList{},
			&NodeClaim{},
			&NodeClaimList{},
		)
		return nil
	})
)
