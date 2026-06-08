package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	clientcmd "k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const defaultAssumeRoleSessionName = "cloudpilotai-terraform"

var shellCredentialExecEnvNames = map[string]struct{}{
	"AWS_PROFILE":           {},
	"AWS_ACCESS_KEY_ID":     {},
	"AWS_SECRET_ACCESS_KEY": {},
	"AWS_SESSION_TOKEN":     {},
}

type ExecutionAuthConfig struct {
	Profile               string
	AssumeRoleARN         string
	AssumeRoleSessionName string
}

func (c ExecutionAuthConfig) HasAssumeRole() bool {
	return c.AssumeRoleARN != ""
}

func (c ExecutionAuthConfig) SessionName() string {
	if c.AssumeRoleSessionName != "" {
		return c.AssumeRoleSessionName
	}
	return defaultAssumeRoleSessionName
}

type assumeRoleResponse struct {
	Credentials struct {
		AccessKeyID     string `json:"AccessKeyId"`
		SecretAccessKey string `json:"SecretAccessKey"`
		SessionToken    string `json:"SessionToken"`
	} `json:"Credentials"`
}

var runAWSCommand = func(ctx context.Context, env map[string]string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "aws", args...)
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("aws %s failed: %w, output: %s", strings.Join(args, " "), err, string(output))
	}

	return output, nil
}

func CommandEnv(ctx context.Context, cfg ExecutionAuthConfig) (map[string]string, error) {
	if !cfg.HasAssumeRole() {
		if cfg.Profile == "" {
			return map[string]string{}, nil
		}
		return map[string]string{"AWS_PROFILE": cfg.Profile}, nil
	}

	args := []string{
		"sts", "assume-role",
		"--role-arn", cfg.AssumeRoleARN,
		"--role-session-name", cfg.SessionName(),
		"--output", "json",
	}
	if cfg.Profile != "" {
		args = append(args, "--profile", cfg.Profile)
	}

	output, err := runAWSCommand(ctx, nil, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to assume AWS role: %w", err)
	}

	var response assumeRoleResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("failed to parse assume-role response: %w", err)
	}

	return map[string]string{
		"AWS_ACCESS_KEY_ID":     response.Credentials.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": response.Credentials.SecretAccessKey,
		"AWS_SESSION_TOKEN":     response.Credentials.SessionToken,
	}, nil
}

func BuildUpdateKubeconfigArgs(clusterName, region, kubeconfigPath string, cfg ExecutionAuthConfig) []string {
	args := []string{
		"eks", "update-kubeconfig",
		"--name", clusterName,
		"--region", region,
		"--kubeconfig", kubeconfigPath,
	}
	if cfg.Profile != "" {
		args = append(args, "--profile", cfg.Profile)
	}
	if cfg.HasAssumeRole() {
		args = append(args,
			"--assume-role-arn", cfg.AssumeRoleARN,
			"--role-arn", cfg.AssumeRoleARN,
		)
	}

	return args
}

func PatchKubeconfigExecEnv(kubeconfigPath string, envVars map[string]string) error {
	cfg, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig %s: %w", kubeconfigPath, err)
	}

	contextConfig := cfg.Contexts[cfg.CurrentContext]
	if contextConfig == nil {
		return fmt.Errorf("current context %q not found in kubeconfig", cfg.CurrentContext)
	}

	authInfo := cfg.AuthInfos[contextConfig.AuthInfo]
	if authInfo == nil {
		return fmt.Errorf("auth info %q not found in kubeconfig", contextConfig.AuthInfo)
	}
	if authInfo.Exec == nil {
		return fmt.Errorf("auth info %q does not use exec auth", contextConfig.AuthInfo)
	}

	merged := make(map[string]string, len(authInfo.Exec.Env)+len(envVars))
	for _, envVar := range authInfo.Exec.Env {
		merged[envVar.Name] = envVar.Value
	}
	for key, value := range envVars {
		merged[key] = value
	}

	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)

	authInfo.Exec.Env = make([]clientcmdapi.ExecEnvVar, 0, len(names))
	for _, name := range names {
		authInfo.Exec.Env = append(authInfo.Exec.Env, clientcmdapi.ExecEnvVar{
			Name:  name,
			Value: merged[name],
		})
	}

	if err := clientcmd.WriteToFile(*cfg, kubeconfigPath); err != nil {
		return fmt.Errorf("failed to write kubeconfig %s: %w", kubeconfigPath, err)
	}

	return nil
}

func KubeconfigForCommandEnv(kubeconfigPath string, env map[string]string) (string, func(), error) {
	noop := func() {}
	if env["AWS_ACCESS_KEY_ID"] == "" || env["AWS_SECRET_ACCESS_KEY"] == "" || env["AWS_SESSION_TOKEN"] == "" {
		return kubeconfigPath, noop, nil
	}

	cfg, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return "", noop, fmt.Errorf("failed to load kubeconfig %s: %w", kubeconfigPath, err)
	}

	contextConfig := cfg.Contexts[cfg.CurrentContext]
	if contextConfig == nil {
		return "", noop, fmt.Errorf("current context %q not found in kubeconfig", cfg.CurrentContext)
	}

	authInfo := cfg.AuthInfos[contextConfig.AuthInfo]
	if authInfo == nil {
		return "", noop, fmt.Errorf("auth info %q not found in kubeconfig", contextConfig.AuthInfo)
	}
	if authInfo.Exec == nil {
		return "", noop, fmt.Errorf("auth info %q does not use exec auth", contextConfig.AuthInfo)
	}

	authInfo.Exec.Args = stripKubeconfigExecAuthArgs(authInfo.Exec.Args)
	authInfo.Exec.Env = stripKubeconfigExecCredentialEnv(authInfo.Exec.Env)

	tempFile, err := os.CreateTemp("", "cloudpilotai-kubeconfig-*")
	if err != nil {
		return "", noop, fmt.Errorf("failed to create temporary kubeconfig: %w", err)
	}
	tempPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return "", noop, fmt.Errorf("failed to close temporary kubeconfig %s: %w", tempPath, err)
	}

	if err := clientcmd.WriteToFile(*cfg, tempPath); err != nil {
		_ = os.Remove(tempPath)
		return "", noop, fmt.Errorf("failed to write temporary kubeconfig %s: %w", tempPath, err)
	}

	return tempPath, func() { _ = os.Remove(tempPath) }, nil
}

func stripKubeconfigExecAuthArgs(args []string) []string {
	stripFlags := map[string]struct{}{
		"--role-arn":        {},
		"--assume-role-arn": {},
		"--role":            {},
		"--profile":         {},
	}

	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if _, ok := stripFlags[arg]; ok {
			if i+1 < len(args) {
				i++
			}
			continue
		}

		strip := false
		for flag := range stripFlags {
			if strings.HasPrefix(arg, flag+"=") {
				strip = true
				break
			}
		}
		if strip {
			continue
		}

		out = append(out, arg)
	}

	return out
}

func stripKubeconfigExecCredentialEnv(env []clientcmdapi.ExecEnvVar) []clientcmdapi.ExecEnvVar {
	out := make([]clientcmdapi.ExecEnvVar, 0, len(env))
	for _, envVar := range env {
		if _, ok := shellCredentialExecEnvNames[envVar.Name]; ok {
			continue
		}
		out = append(out, envVar)
	}

	return out
}
