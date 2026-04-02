// Package aws provides utilities for interacting with AWS.
package aws

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

type GetCallerIdentityResponse struct {
	UserID  string `json:"UserId"`
	Account string `json:"Account"`
	Arn     string `json:"Arn"`
}

func GetAccountID(profile string) (string, error) {
	args := []string{"sts", "get-caller-identity"}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	cmd := exec.Command("aws", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get account ID: %w, output: %s", err, string(output))
	}

	var response GetCallerIdentityResponse
	if err = json.Unmarshal(output, &response); err != nil {
		return "", err
	}

	return response.Account, nil
}
