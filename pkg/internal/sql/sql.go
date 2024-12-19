package sql

import (
	"strings"
)

// EscapeBacktick escapes the ` characted in strings to make them safe for use in SQL queries as literal values.
func EscapeBacktick(s string) string {
	return strings.ReplaceAll(s, "`", "\\`")
}

// EscapeSingleQuote escapes the ' characted in strings to make them safe for use in SQL queries as string values.
func EscapeSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
}
