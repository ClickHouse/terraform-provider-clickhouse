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
	StateRunning      = "running"
	StateIdle         = "idle"
	StateAwaking      = "awaking"

	serviceStateCommandAwake = "awake"

	ResponseHeaderRateLimitReset = "X-RateLimit-Reset"

	ComplianceTypeHIPAA = "hipaa"
	ComplianceTypePCI   = "pci"

	AutoscalingModeVertical   = "vertical"
	AutoscalingModeHorizontal = "horizontal"

	// ProfileGen2 is the instance profile of a service on Gen 2 hardware.
	ProfileGen2 = "v2-default"
)
