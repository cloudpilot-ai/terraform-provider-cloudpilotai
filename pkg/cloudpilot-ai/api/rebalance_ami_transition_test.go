package api

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestEC2ExplicitAliasReplacesPreviousSelectorsWithoutMutatingBase(t *testing.T) {
	const payload = `{"name":"example","futureClass":true,"nodeClassSpec":{"role":"example-role","amiFamily":"Bottlerocket","amiSelectorTerms":[{"id":"ami-example","futureSelector":true}]}}`
	var current EC2NodeClass
	if err := json.Unmarshal([]byte(payload), &current); err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	model := EC2NodeClassModel{Name: types.StringValue("example"), AmiAlias: types.StringValue("al2023@latest")}
	got, err := model.ToEC2NodeClass(context.Background(), "example", current, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.NodeClassSpec.AMISelectorTerms) != 1 || got.NodeClassSpec.AMISelectorTerms[0].Alias != "al2023@latest" || got.NodeClassSpec.AMISelectorTerms[0].ID != "" || got.NodeClassSpec.AMIFamily != nil {
		t.Fatalf("explicit alias retained competing selectors or family: %#v", got.NodeClassSpec)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"id":"ami-example"`) || strings.Contains(string(encoded), `"amiFamily"`) || !strings.Contains(string(encoded), `"futureClass":true`) {
		t.Fatalf("serialized alias update = %s", encoded)
	}
	after, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("alias update mutated its base: before %s, after %s", before, after)
	}
}

func TestEC2AliasPreservesCompatibleFamilies(t *testing.T) {
	for _, tc := range []struct {
		alias, family string
		clearFamily   bool
	}{
		{alias: "al2@latest", family: "AL2"},
		{alias: "al2023@latest", family: "AL2023"},
		{alias: "bottlerocket@latest", family: "Bottlerocket"},
		{alias: "windows2019@latest", family: "Windows2019"},
		{alias: "windows2022@latest", family: "Windows2022"},
		{alias: "al2023@latest", family: "Custom"},
		{alias: "al2023@latest", family: "Bottlerocket", clearFamily: true},
	} {
		t.Run(tc.alias+"/"+tc.family, func(t *testing.T) {
			family := tc.family
			current := EC2NodeClass{NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{AMIFamily: &family}}
			applyAmiAlias(&current, types.StringValue(tc.alias))
			if tc.clearFamily {
				if current.NodeClassSpec.AMIFamily != nil {
					t.Fatalf("incompatible AMI family retained: %#v", current.NodeClassSpec.AMIFamily)
				}
			} else if current.NodeClassSpec.AMIFamily == nil || *current.NodeClassSpec.AMIFamily != tc.family {
				t.Fatalf("compatible AMI family lost: %#v", current.NodeClassSpec.AMIFamily)
			}
		})
	}
	current := EC2NodeClass{NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{}}
	applyAmiAlias(&current, types.StringValue("al2023@latest"))
	if current.NodeClassSpec.AMIFamily != nil {
		t.Fatal("alias update introduced an explicit AMI family")
	}
}

func TestEC2UnmanagedAliasPreservesExistingSpec(t *testing.T) {
	for _, alias := range []types.String{types.StringNull(), types.StringUnknown()} {
		const payload = `{"name":"example","nodeClassSpec":{"role":"example-role","amiFamily":"Bottlerocket","amiSelectorTerms":[{"id":"ami-example"}],"futureSpec":true}}`
		var current EC2NodeClass
		if err := json.Unmarshal([]byte(payload), &current); err != nil {
			t.Fatal(err)
		}
		before, err := json.Marshal(current)
		if err != nil {
			t.Fatal(err)
		}
		model := EC2NodeClassModel{Name: types.StringValue("example"), AmiAlias: alias}
		got, err := model.ToEC2NodeClass(context.Background(), "example", current, nil)
		if err != nil {
			t.Fatal(err)
		}
		after, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatalf("unmanaged alias changed existing spec: before %s, after %s", before, after)
		}
	}
}

func TestEC2EmptyAliasRetainsRemovalSemantics(t *testing.T) {
	current := EC2NodeClass{NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{AMISelectorTerms: []awsproviderv1.AMISelectorTerm{{Alias: "al2023@latest"}}}}
	applyAmiAlias(&current, types.StringValue(""))
	if len(current.NodeClassSpec.AMISelectorTerms) != 0 {
		t.Fatalf("empty alias retained alias selectors: %#v", current.NodeClassSpec.AMISelectorTerms)
	}
	current.NodeClassSpec.AMISelectorTerms = []awsproviderv1.AMISelectorTerm{{ID: "ami-example"}}
	applyAmiAlias(&current, types.StringValue(""))
	if len(current.NodeClassSpec.AMISelectorTerms) != 1 || current.NodeClassSpec.AMISelectorTerms[0].ID != "ami-example" {
		t.Fatalf("empty alias removed a non-alias selector: %#v", current.NodeClassSpec.AMISelectorTerms)
	}
}

func TestEC2RoleTransitionRequiresRecreation(t *testing.T) {
	profile := "existing-profile"
	current := EC2NodeClass{Name: "example", NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{InstanceProfile: &profile}}
	for _, tc := range []struct {
		name     string
		model    EC2NodeClassModel
		template *EC2NodeClassTemplateModel
	}{
		{name: "direct", model: EC2NodeClassModel{Role: types.StringValue("new-role")}},
		{name: "template", template: &EC2NodeClassTemplateModel{Role: types.StringValue("new-role")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.model.ToEC2NodeClass(context.Background(), "example", current, tc.template)
			if err == nil || !strings.Contains(err.Error(), "cannot switch from instanceProfile to role in place") || !strings.Contains(err.Error(), "recreating") {
				t.Fatalf("role transition error = %v", err)
			}
			if current.NodeClassSpec.Role != "" || current.NodeClassSpec.InstanceProfile == nil || *current.NodeClassSpec.InstanceProfile != profile {
				t.Fatal("rejected role transition mutated the existing class")
			}
		})
	}
}

func TestEC2RoleGuardAllowsUnmanagedRoleAndNewOriginClass(t *testing.T) {
	ctx := context.Background()
	profile := "existing-profile"
	current := EC2NodeClass{Name: "example", NodeClassSpec: &awsproviderv1.EC2NodeClassSpec{InstanceProfile: &profile}}
	for _, role := range []types.String{types.StringNull(), types.StringUnknown(), types.StringValue("")} {
		model := EC2NodeClassModel{Name: types.StringValue("example"), Role: role}
		if _, err := model.ToEC2NodeClass(ctx, "example", current, nil); err != nil {
			t.Fatalf("unmanaged role rejected: %v", err)
		}
	}
	model := EC2NodeClassModel{Name: types.StringValue("new"), Role: types.StringValue("new-role")}
	if _, err := model.ToEC2NodeClass(ctx, "example", EC2NodeClass{Name: "new"}, nil); err != nil {
		t.Fatalf("new typed class rejected: %v", err)
	}
	model.OriginNodeClassJSON = types.StringValue(`{"name":"new","nodeClassSpec":{"instanceProfile":"new-profile"}}`)
	got, err := model.ToEC2NodeClass(ctx, "example", EC2NodeClass{}, nil)
	if err != nil || got == nil || got.NodeClassSpec.InstanceProfile == nil || *got.NodeClassSpec.InstanceProfile != "new-profile" || got.NodeClassSpec.Role != "" {
		t.Fatalf("new origin class rejected or changed: %#v, error = %v", got, err)
	}
}
