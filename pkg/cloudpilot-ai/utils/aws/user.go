// Package aws provides utilities for interacting with AWS.
package aws

import (
	"context"
	"encoding/json"
	"fmt"
)

type GetCallerIdentityResponse struct {
	UserID  string `json:"UserId"`
	Account string `json:"Account"`
	Arn     string `json:"Arn"`
}

func GetAccountID(ctx context.Context, auth ExecutionAuthConfig) (string, error) {
	env, err := CommandEnv(ctx, auth)
	if err != nil {
		return "", fmt.Errorf("failed to build AWS command env: %w", err)
	}

	output, err := runAWSCommand(ctx, env, "sts", "get-caller-identity", "--output", "json")
	if err != nil {
		return "", fmt.Errorf("failed to get account ID: %w", err)
	}

	var response GetCallerIdentityResponse
	if err = json.Unmarshal(output, &response); err != nil {
		return "", fmt.Errorf("failed to parse get-caller-identity response: %w", err)
	}

	return response.Account, nil
}
