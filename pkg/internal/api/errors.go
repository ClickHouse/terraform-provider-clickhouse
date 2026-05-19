package api

import (
	"strings"
)

func IsNotFound(err error) bool {
	if err == nil {
		return false
	}

	return strings.HasPrefix(err.Error(), "status: 404")
}

func is5xx(err error) bool {
	if err == nil {
		return false
	}

	return strings.HasPrefix(err.Error(), "status: 5")
}

func IsForbidden(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(err.Error(), "status: 403")
}

func DescribePostgresAPIError(err error) string {
	if err == nil {
		return ""
	}

	if IsForbidden(err) {
		return err.Error() + ". Managed Postgres requires the FT_ORG_MANAGED_POSTGRES_SERVICES feature flag to be enabled for this organization."
	}

	return err.Error()
}
