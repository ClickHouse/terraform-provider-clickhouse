package clickstack

// Attribute names shared across the ClickStack resource and data source schemas.
// Extracted from the standalone provider's provider.go when the group was ported
// into this provider.
const (
	idAttr             = "id"
	teamAttr           = "team"
	nameAttr           = "name"
	passwordAttr       = "password"
	descriptionAttr    = "description"
	dashboardJSONAttr  = "dashboard_json"
	normalizedJSONAttr = "normalized_json"
	emailAttr          = "email"
	roleIDAttr         = "role_id"
	statusAttr         = "status"
	inviteURLAttr      = "invite_url"
)
