package api

const (
	// SSHKeyStatusPending is the initial status of a freshly created SSH key
	// resource, before connectivity has been validated.
	SSHKeyStatusPending = "pending"
	// SSHKeyStatusActive indicates the last validation succeeded.
	SSHKeyStatusActive = "active"
	// SSHKeyStatusFailed indicates the last validation failed.
	SSHKeyStatusFailed = "failed"
)

var SSHKeyStatuses = []string{
	SSHKeyStatusPending,
	SSHKeyStatusActive,
	SSHKeyStatusFailed,
}

// CreateSSHKey is the request payload for creating an SSH key resource. The
// server generates the Ed25519 keypair and returns the public key to install
// on the bastion; the private key never crosses the API boundary.
type CreateSSHKey struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
}

// SSHKey represents a standalone, service-scoped SSH key resource.
type SSHKey struct {
	ID              string  `json:"id,omitempty"`
	ServiceID       string  `json:"serviceId,omitempty"`
	Name            string  `json:"name,omitempty"`
	Description     string  `json:"description,omitempty"`
	Host            string  `json:"host,omitempty"`
	Port            int     `json:"port,omitempty"`
	Username        string  `json:"username,omitempty"`
	PublicKey       string  `json:"publicKey,omitempty"`
	Status          string  `json:"status,omitempty"`
	StatusMessage   string  `json:"statusMessage,omitempty"`
	LastValidatedAt *string `json:"lastValidatedAt,omitempty"`
	CreatedAt       string  `json:"createdAt,omitempty"`
	UpdatedAt       string  `json:"updatedAt,omitempty"`
}
