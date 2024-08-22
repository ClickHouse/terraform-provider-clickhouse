package api

import (
	"time"
)

const (
	maxRetries = 5

	BackoffInitialInterval = 5 * time.Second
	BackoffMaxElapsed      = 81 * time.Second
	BackoffMultiplier      = 2

	TierDevelopment = "development"
	TierProduction  = "production"

	StatusProvisioning = "provisioning"
	StatusStopped      = "stopped"
	StatusStopping     = "stopping"
)
