package api

const (
	TierDevelopment = "development"
	TierProduction  = "production"
	TierPPv2        = ""

	ReleaseChannelDefault = "default"
	ReleaseChannelFast    = "fast"
	ReleaseChannelSlow    = "slow"

	StateProvisioning     = "provisioning"
	StateStopped          = "stopped"
	StateStopping         = "stopping"
	StateRunning          = "running"
	StateStarting         = "starting"
	StateIdle             = "idle"
	StateAwaking          = "awaking"
	StateDegraded         = "degraded"
	StatePartiallyRunning = "partially_running"

	ServiceStateCommandStart = "start"
	ServiceStateCommandStop  = "stop"

	ResponseHeaderRateLimitReset = "X-RateLimit-Reset"

	ComplianceTypeHIPAA = "hipaa"
	ComplianceTypePCI   = "pci"
)
