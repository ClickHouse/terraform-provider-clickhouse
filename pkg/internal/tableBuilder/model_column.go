package tableBuilder

import (
	"fmt"
)

type Column struct {
	Name         string
	Type         string
	Nullable     bool
	Default      string
	Materialized string
	Ephemeral    bool
	Alias        string
	Codec        string
	Comment      string
	TTL          *TTL
}

type TTL struct {
	TimeColumn string
	Interval   string
}

func (c *Column) querySpec() string {
	col := c.Type
	if c.Nullable {
		col = fmt.Sprintf("Nullable(%s)", c.Type)
	}
	if c.Default != "" {
		col = fmt.Sprintf("%s DEFAULT %s", col, c.Default)
	}
	if c.Materialized != "" {
		col = fmt.Sprintf("%s MATERIALIZED %s", col, c.Materialized)
	}
	if c.Alias != "" {
		col = fmt.Sprintf("%s ALIAS %s", col, c.Alias)
	}
	if c.Comment != "" {
		col = fmt.Sprintf("%s COMMENT '%s'", col, c.Comment)
	}
	if c.Codec != "" {
		col = fmt.Sprintf("%s CODEC(%s)", col, c.Codec)
	}
	if c.TTL != nil {
		col = fmt.Sprintf("%s TTL %s + INTERVAL %s", col, c.TTL.TimeColumn, c.TTL.Interval)
	}
	if c.Ephemeral {
		col = fmt.Sprintf("%s EPHEMERAL", col)
	}
	return fmt.Sprintf("%s %s", c.Name, col)
}
