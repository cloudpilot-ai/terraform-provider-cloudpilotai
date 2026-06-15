package helper

import (
	"strings"
	"testing"
)

func TestNewManagedScriptOverrideConfigIncludesOnlyTerraformManagedKeys(t *testing.T) {
	cfg := NewManagedScriptOverrideConfig(
		map[string]string{"CUSTOM_NODE_ROLE": "terraform-role"},
		map[string]string{"AWS_PROFILE": "terraform-profile"},
	)

	if _, ok := cfg.StripEnvNames["AWS_PROFILE"]; !ok {
		t.Fatalf("StripEnvNames missing AWS_PROFILE: %#v", cfg.StripEnvNames)
	}
	if _, ok := cfg.StripEnvNames["AWS_DEFAULT_PROFILE"]; !ok {
		t.Fatalf("StripEnvNames missing AWS_DEFAULT_PROFILE: %#v", cfg.StripEnvNames)
	}
	if _, ok := cfg.StripEnvNames["CUSTOM_NODE_ROLE"]; !ok {
		t.Fatalf("StripEnvNames missing CUSTOM_NODE_ROLE: %#v", cfg.StripEnvNames)
	}
	if _, ok := cfg.StripEnvNames["AWS_ACCESS_KEY_ID"]; ok {
		t.Fatalf("StripEnvNames should not include assume-role credentials when only aws_profile is configured: %#v", cfg.StripEnvNames)
	}
	if !cfg.StripAWSProfileFlag {
		t.Fatalf("StripAWSProfileFlag = false, want true")
	}
}

func TestNewManagedScriptOverrideConfigTreatsAWSDefaultProfileAsManaged(t *testing.T) {
	cfg := NewManagedScriptOverrideConfig(
		nil,
		map[string]string{"AWS_DEFAULT_PROFILE": "terraform-default-profile"},
	)

	if _, ok := cfg.StripEnvNames["AWS_DEFAULT_PROFILE"]; !ok {
		t.Fatalf("StripEnvNames missing AWS_DEFAULT_PROFILE: %#v", cfg.StripEnvNames)
	}
	if cfg.StripAWSProfileFlag != true {
		t.Fatalf("StripAWSProfileFlag = %v, want true", cfg.StripAWSProfileFlag)
	}
}

func TestNormalizeManagedScriptStripsTerraformManagedAssignmentsAndAWSProfileArgs(t *testing.T) {
	script := strings.Join([]string{
		`export AWS_PROFILE=manual-profile`,
		`AWS_DEFAULT_PROFILE=legacy aws sts get-caller-identity --profile legacy`,
		`CUSTOM_NODE_ROLE=manual-role kubectl get nodes`,
		`FOO=bar aws ec2 describe-subnets --profile legacy`,
		`echo keep-me`,
	}, "\n")
	want := strings.Join([]string{
		``,
		`aws sts get-caller-identity`,
		`kubectl get nodes`,
		`FOO=bar aws ec2 describe-subnets`,
		`echo keep-me`,
	}, "\n")

	got := NormalizeManagedScript(script, ManagedScriptOverrideConfig{
		StripEnvNames: map[string]struct{}{
			"AWS_PROFILE":         {},
			"AWS_DEFAULT_PROFILE": {},
			"CUSTOM_NODE_ROLE":    {},
		},
		StripAWSProfileFlag: true,
	})

	if got != want {
		t.Fatalf("NormalizeManagedScript() = %q, want %q", got, want)
	}
}

func TestNormalizeManagedScriptStripsAssumeRoleCredentialEnvAndManagedUnsetLines(t *testing.T) {
	script := strings.Join([]string{
		`unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN KEEP_ME`,
		`export AWS_ACCESS_KEY_ID=manual-access-key`,
		`AWS_ROLE_ARN=arn:aws:iam::123456789012:role/manual aws sts get-caller-identity`,
	}, "\n")

	got := NormalizeManagedScript(script, ManagedScriptOverrideConfig{
		StripEnvNames: map[string]struct{}{
			"AWS_PROFILE":                 {},
			"AWS_DEFAULT_PROFILE":         {},
			"AWS_ACCESS_KEY_ID":           {},
			"AWS_SECRET_ACCESS_KEY":       {},
			"AWS_SESSION_TOKEN":           {},
			"AWS_ROLE_ARN":                {},
			"AWS_WEB_IDENTITY_TOKEN_FILE": {},
		},
		StripAWSProfileFlag: true,
	})

	if strings.Contains(got, "manual-access-key") {
		t.Fatalf("normalized script still contains stripped access key assignment: %q", got)
	}
	if strings.Contains(got, "AWS_ROLE_ARN=") {
		t.Fatalf("normalized script still contains stripped AWS_ROLE_ARN assignment: %q", got)
	}
	if strings.Contains(got, "unset AWS_ACCESS_KEY_ID") {
		t.Fatalf("normalized script should remove managed unset names: %q", got)
	}
	if !strings.Contains(got, "unset KEEP_ME") {
		t.Fatalf("normalized script should preserve unrelated unset names: %q", got)
	}
	if !strings.Contains(got, "aws sts get-caller-identity") {
		t.Fatalf("normalized script lost aws command body: %q", got)
	}
}

func TestNewManagedScriptOverrideConfigAlwaysStripsAWSProfileOverrides(t *testing.T) {
	cfg := NewManagedScriptOverrideConfig(nil, map[string]string{})

	if _, ok := cfg.StripEnvNames["AWS_PROFILE"]; !ok {
		t.Fatalf("StripEnvNames missing AWS_PROFILE: %#v", cfg.StripEnvNames)
	}
	if _, ok := cfg.StripEnvNames["AWS_DEFAULT_PROFILE"]; !ok {
		t.Fatalf("StripEnvNames missing AWS_DEFAULT_PROFILE: %#v", cfg.StripEnvNames)
	}
	if !cfg.StripAWSProfileFlag {
		t.Fatalf("StripAWSProfileFlag = false, want true")
	}
}

func TestNormalizeManagedScriptStripsProfileOverridesWhenTerraformDidNotConfigureAWSAuth(t *testing.T) {
	script := strings.Join([]string{
		`export AWS_PROFILE=manual`,
		`aws sts get-caller-identity --profile manual`,
	}, "\n")
	want := strings.Join([]string{
		``,
		`aws sts get-caller-identity`,
	}, "\n")

	got := NormalizeManagedScript(script, NewManagedScriptOverrideConfig(nil, map[string]string{}))

	if got != want {
		t.Fatalf("NormalizeManagedScript() = %q, want %q", got, want)
	}
}

func TestNormalizeManagedScriptKeepsManagedContinuationAssignmentLineIntact(t *testing.T) {
	script := strings.Join([]string{
		`AWS_PROFILE=manual-profile \`,
		`  aws sts get-caller-identity --profile manual-profile`,
		`echo keep-me`,
	}, "\n")
	want := strings.Join([]string{
		`AWS_PROFILE=manual-profile \`,
		`  aws sts get-caller-identity`,
		`echo keep-me`,
	}, "\n")

	got := NormalizeManagedScript(script, ManagedScriptOverrideConfig{
		StripEnvNames: map[string]struct{}{
			"AWS_PROFILE":         {},
			"AWS_DEFAULT_PROFILE": {},
		},
		StripAWSProfileFlag: true,
	})

	if got != want {
		t.Fatalf("NormalizeManagedScript() = %q, want %q", got, want)
	}
}
