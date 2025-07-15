package api

const (
	TierDevelopment = "development"
	TierProduction  = "production"
	TierPPv2        = ""

	ReleaseChannelDefault = "default"
	ReleaseChannelFast    = "fast"
	ReleaseChannelSlow    = "slow"

	StateProvisioning = "provisioning"
	StateStopped      = "stopped"
	StateStopping     = "stopping"

	ResponseHeaderRateLimitReset = "X-RateLimit-Reset"
)
