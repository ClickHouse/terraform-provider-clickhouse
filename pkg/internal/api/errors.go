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
