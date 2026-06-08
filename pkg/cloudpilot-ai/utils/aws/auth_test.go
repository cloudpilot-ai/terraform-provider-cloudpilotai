package aws

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	clientcmd "k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestCommandEnvReturnsProfileFallbackWhenAssumeRoleIsUnset(t *testing.T) {
	got, err := CommandEnv(context.Background(), ExecutionAuthConfig{Profile: "dev"})
	if err != nil {
		t.Fatalf("CommandEnv() error = %v", err)
	}

	want := map[string]string{"AWS_PROFILE": "dev"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CommandEnv() = %#v, want %#v", got, want)
	}
}

func TestCommandEnvAssumeRoleParsesTemporaryCredentials(t *testing.T) {
	previous := runAWSCommand
	t.Cleanup(func() { runAWSCommand = previous })

	runAWSCommand = func(_ context.Context, env map[string]string, args ...string) ([]byte, error) {
		if len(env) != 0 {
			t.Fatalf("assume-role bootstrap env = %#v, want empty override env", env)
		}
		wantArgs := []string{
			"sts", "assume-role",
			"--role-arn", "arn:aws:iam::123456789012:role/sts-admin",
			"--role-session-name", "cloudpilotai-terraform",
			"--output", "json",
			"--profile", "dev",
		}
		if !reflect.DeepEqual(args, wantArgs) {
			t.Fatalf("args = %#v, want %#v", args, wantArgs)
		}

		return []byte(`{"Credentials":{"AccessKeyId":"AKIA_TEST","SecretAccessKey":"secret","SessionToken":"token"}}`), nil
	}

	got, err := CommandEnv(context.Background(), ExecutionAuthConfig{
		Profile:       "dev",
		AssumeRoleARN: "arn:aws:iam::123456789012:role/sts-admin",
	})
	if err != nil {
		t.Fatalf("CommandEnv() error = %v", err)
	}

	if got["AWS_ACCESS_KEY_ID"] != "AKIA_TEST" {
		t.Fatalf("AWS_ACCESS_KEY_ID = %q, want AKIA_TEST", got["AWS_ACCESS_KEY_ID"])
	}
	if got["AWS_SECRET_ACCESS_KEY"] != "secret" {
		t.Fatalf("AWS_SECRET_ACCESS_KEY = %q, want secret", got["AWS_SECRET_ACCESS_KEY"])
	}
	if got["AWS_SESSION_TOKEN"] != "token" {
		t.Fatalf("AWS_SESSION_TOKEN = %q, want token", got["AWS_SESSION_TOKEN"])
	}
	if _, ok := got["AWS_PROFILE"]; ok {
		t.Fatalf("AWS_PROFILE should not leak into assumed-role env, got %#v", got)
	}
}

func TestBuildUpdateKubeconfigArgsIncludesProfileAndRoleFlags(t *testing.T) {
	got := BuildUpdateKubeconfigArgs("demo", "us-east-2", "/tmp/demo-kubeconfig", ExecutionAuthConfig{
		Profile:               "dev",
		AssumeRoleARN:         "arn:aws:iam::123456789012:role/sts-admin",
		AssumeRoleSessionName: "terraform-user",
	})

	want := []string{
		"eks", "update-kubeconfig",
		"--name", "demo",
		"--region", "us-east-2",
		"--kubeconfig", "/tmp/demo-kubeconfig",
		"--profile", "dev",
		"--assume-role-arn", "arn:aws:iam::123456789012:role/sts-admin",
		"--role-arn", "arn:aws:iam::123456789012:role/sts-admin",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildUpdateKubeconfigArgs() = %#v, want %#v", got, want)
	}
}

func TestPatchKubeconfigExecEnvInjectsAWSProfileIntoCurrentContext(t *testing.T) {
	path := t.TempDir() + "/kubeconfig"
	cfg := clientcmdapi.NewConfig()
	cfg.CurrentContext = "demo"
	cfg.Contexts["demo"] = &clientcmdapi.Context{Cluster: "demo", AuthInfo: "demo-user"}
	cfg.Clusters["demo"] = &clientcmdapi.Cluster{Server: "https://example.invalid"}
	cfg.AuthInfos["demo-user"] = &clientcmdapi.AuthInfo{
		Exec: &clientcmdapi.ExecConfig{
			Command: "aws",
			Args:    []string{"eks", "get-token", "--cluster-name", "demo"},
		},
	}
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		t.Fatalf("WriteToFile() error = %v", err)
	}

	if err := PatchKubeconfigExecEnv(path, map[string]string{"AWS_PROFILE": "dev"}); err != nil {
		t.Fatalf("PatchKubeconfigExecEnv() error = %v", err)
	}

	patched, err := clientcmd.LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	authInfo := patched.AuthInfos["demo-user"]
	if authInfo == nil || authInfo.Exec == nil {
		t.Fatalf("patched kubeconfig missing exec auth")
	}
	if len(authInfo.Exec.Env) != 1 {
		t.Fatalf("exec env length = %d, want 1", len(authInfo.Exec.Env))
	}
	if authInfo.Exec.Env[0].Name != "AWS_PROFILE" || authInfo.Exec.Env[0].Value != "dev" {
		t.Fatalf("exec env = %#v, want AWS_PROFILE=dev", authInfo.Exec.Env)
	}
}

func TestPatchKubeconfigExecEnvMergesOverridesAndSortsExecEnv(t *testing.T) {
	path := t.TempDir() + "/kubeconfig"
	cfg := clientcmdapi.NewConfig()
	cfg.CurrentContext = "demo"
	cfg.Contexts["demo"] = &clientcmdapi.Context{Cluster: "demo", AuthInfo: "demo-user"}
	cfg.Clusters["demo"] = &clientcmdapi.Cluster{Server: "https://example.invalid"}
	cfg.AuthInfos["demo-user"] = &clientcmdapi.AuthInfo{
		Exec: &clientcmdapi.ExecConfig{
			Command: "aws",
			Args:    []string{"eks", "get-token", "--cluster-name", "demo"},
			Env: []clientcmdapi.ExecEnvVar{
				{Name: "AWS_REGION", Value: "us-east-2"},
				{Name: "AWS_PROFILE", Value: "old"},
			},
		},
	}
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		t.Fatalf("WriteToFile() error = %v", err)
	}

	err := PatchKubeconfigExecEnv(path, map[string]string{
		"AWS_PROFILE":       "dev",
		"AWS_SESSION_TOKEN": "token",
		"AWS_ACCESS_KEY_ID": "AKIA_TEST",
	})
	if err != nil {
		t.Fatalf("PatchKubeconfigExecEnv() error = %v", err)
	}

	patched, err := clientcmd.LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	got := patched.AuthInfos["demo-user"].Exec.Env
	want := []clientcmdapi.ExecEnvVar{
		{Name: "AWS_ACCESS_KEY_ID", Value: "AKIA_TEST"},
		{Name: "AWS_PROFILE", Value: "dev"},
		{Name: "AWS_REGION", Value: "us-east-2"},
		{Name: "AWS_SESSION_TOKEN", Value: "token"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("exec env = %#v, want %#v", got, want)
	}
}

func TestPatchKubeconfigExecEnvWrapsLoadErrorsWithPath(t *testing.T) {
	path := t.TempDir() + "/missing-kubeconfig"

	err := PatchKubeconfigExecEnv(path, map[string]string{"AWS_PROFILE": "dev"})
	if err == nil {
		t.Fatalf("PatchKubeconfigExecEnv() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "failed to load kubeconfig "+path) {
		t.Fatalf("PatchKubeconfigExecEnv() error = %q, want load context with path", err)
	}
}

func TestKubeconfigForCommandEnvWithTemporaryCredentialsStripsExecRoleAndCredentialConfig(t *testing.T) {
	path := t.TempDir() + "/kubeconfig"
	originalArgs := []string{
		"eks", "get-token",
		"--cluster-name", "demo",
		"--profile", "dev",
		"--role", "arn:aws:iam::123456789012:role/sts-admin",
		"--role-arn", "arn:aws:iam::123456789012:role/sts-admin",
		"--assume-role-arn", "arn:aws:iam::123456789012:role/sts-admin",
		"--region", "us-east-2",
	}
	originalEnv := []clientcmdapi.ExecEnvVar{
		{Name: "AWS_REGION", Value: "us-east-2"},
		{Name: "AWS_PROFILE", Value: "dev"},
		{Name: "AWS_ACCESS_KEY_ID", Value: "OLD_ACCESS_KEY"},
		{Name: "AWS_SECRET_ACCESS_KEY", Value: "old-secret"},
		{Name: "AWS_SESSION_TOKEN", Value: "old-token"},
		{Name: "CUSTOM_ENV", Value: "keep"},
	}
	cfg := clientcmdapi.NewConfig()
	cfg.CurrentContext = "demo"
	cfg.Contexts["demo"] = &clientcmdapi.Context{Cluster: "demo", AuthInfo: "demo-user"}
	cfg.Clusters["demo"] = &clientcmdapi.Cluster{Server: "https://example.invalid"}
	cfg.AuthInfos["demo-user"] = &clientcmdapi.AuthInfo{
		Exec: &clientcmdapi.ExecConfig{
			Command: "aws",
			Args:    originalArgs,
			Env:     originalEnv,
		},
	}
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		t.Fatalf("WriteToFile() error = %v", err)
	}

	gotPath, cleanup, err := KubeconfigForCommandEnv(path, map[string]string{
		"AWS_ACCESS_KEY_ID":     "AKIA_TEST",
		"AWS_SECRET_ACCESS_KEY": "secret",
		"AWS_SESSION_TOKEN":     "token",
	})
	if err != nil {
		t.Fatalf("KubeconfigForCommandEnv() error = %v", err)
	}
	if gotPath == path {
		t.Fatalf("KubeconfigForCommandEnv() path = original path, want temporary copy")
	}
	if _, err := os.Stat(gotPath); err != nil {
		t.Fatalf("temporary kubeconfig missing before cleanup: %v", err)
	}

	patched, err := clientcmd.LoadFromFile(gotPath)
	if err != nil {
		t.Fatalf("LoadFromFile(temp) error = %v", err)
	}
	gotArgs := patched.AuthInfos["demo-user"].Exec.Args
	wantArgs := []string{"eks", "get-token", "--cluster-name", "demo", "--region", "us-east-2"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("exec args = %#v, want %#v", gotArgs, wantArgs)
	}
	gotEnv := patched.AuthInfos["demo-user"].Exec.Env
	wantEnv := []clientcmdapi.ExecEnvVar{
		{Name: "AWS_REGION", Value: "us-east-2"},
		{Name: "CUSTOM_ENV", Value: "keep"},
	}
	if !reflect.DeepEqual(gotEnv, wantEnv) {
		t.Fatalf("exec env = %#v, want %#v", gotEnv, wantEnv)
	}

	original, err := clientcmd.LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile(original) error = %v", err)
	}
	if !reflect.DeepEqual(original.AuthInfos["demo-user"].Exec.Args, originalArgs) {
		t.Fatalf("original exec args changed to %#v, want %#v", original.AuthInfos["demo-user"].Exec.Args, originalArgs)
	}
	if !reflect.DeepEqual(original.AuthInfos["demo-user"].Exec.Env, originalEnv) {
		t.Fatalf("original exec env changed to %#v, want %#v", original.AuthInfos["demo-user"].Exec.Env, originalEnv)
	}

	cleanup()
	if _, err := os.Stat(gotPath); !os.IsNotExist(err) {
		t.Fatalf("temporary kubeconfig still exists after cleanup, stat err = %v", err)
	}
}

func TestKubeconfigForCommandEnvWithProfileOnlyReturnsOriginalPath(t *testing.T) {
	path := t.TempDir() + "/kubeconfig"
	cfg := clientcmdapi.NewConfig()
	cfg.CurrentContext = "demo"
	cfg.Contexts["demo"] = &clientcmdapi.Context{Cluster: "demo", AuthInfo: "demo-user"}
	cfg.Clusters["demo"] = &clientcmdapi.Cluster{Server: "https://example.invalid"}
	cfg.AuthInfos["demo-user"] = &clientcmdapi.AuthInfo{
		Exec: &clientcmdapi.ExecConfig{
			Command: "aws",
			Args:    []string{"eks", "get-token", "--cluster-name", "demo", "--profile", "dev"},
		},
	}
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		t.Fatalf("WriteToFile() error = %v", err)
	}

	gotPath, cleanup, err := KubeconfigForCommandEnv(path, map[string]string{"AWS_PROFILE": "dev"})
	if err != nil {
		t.Fatalf("KubeconfigForCommandEnv() error = %v", err)
	}
	if gotPath != path {
		t.Fatalf("KubeconfigForCommandEnv() path = %q, want original %q", gotPath, path)
	}
	cleanup()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("original kubeconfig missing after no-op cleanup: %v", err)
	}
}

func TestGetAccountIDWrapsJSONParseErrorsWithContext(t *testing.T) {
	previous := runAWSCommand
	t.Cleanup(func() { runAWSCommand = previous })

	runAWSCommand = func(_ context.Context, _ map[string]string, _ ...string) ([]byte, error) {
		return []byte(`not-json`), nil
	}

	_, err := GetAccountID(context.Background(), ExecutionAuthConfig{})
	if err == nil {
		t.Fatalf("GetAccountID() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "failed to parse get-caller-identity response") {
		t.Fatalf("GetAccountID() error = %q, want parse context", err)
	}
}
