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

func RetriableError(statusCode int) bool {
	return statusCode == 429 || statusCode >= 500
}
