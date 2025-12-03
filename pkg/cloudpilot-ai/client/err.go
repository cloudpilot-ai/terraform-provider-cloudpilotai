package client

import (
	"errors"
)

var (
	ErrNotFound = errors.New("resource not found")

	ErrRPInUse    = errors.New("RecommendationPolicy in use")
	ErrRPNotFound = errors.New("RecommendationPolicy not found")
)
