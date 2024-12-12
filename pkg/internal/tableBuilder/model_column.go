package tableBuilder

import (
	"fmt"
)

type Column struct {
	Name         string
	Type         string
	Nullable     bool
	Default      *string
	Materialized *string
	Ephemeral    bool
	Alias        *string
	Comment      *string
}

func (c *Column) querySpec() string {
	col := c.Type
	if c.Nullable {
		col = fmt.Sprintf("Nullable(%s)", c.Type)
	}
	if c.Default != nil {
		col = fmt.Sprintf("%s DEFAULT %s", col, *c.Default)
	}
	if c.Materialized != nil {
		col = fmt.Sprintf("%s MATERIALIZED %s", col, *c.Materialized)
	}
	if c.Alias != nil {
		col = fmt.Sprintf("%s ALIAS %s", col, *c.Alias)
	}
	if c.Comment != nil {
		col = fmt.Sprintf("%s COMMENT '%s'", col, *c.Comment)
	}
	if c.Ephemeral {
		col = fmt.Sprintf("%s EPHEMERAL", col)
	}
	return fmt.Sprintf("%s %s", c.Name, col)
}
