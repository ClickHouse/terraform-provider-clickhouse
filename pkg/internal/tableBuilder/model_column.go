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

func (c *Column) diffQueries(new Column) []string {
	queries := make([]string, 0)

	// Nullable and Type
	if c.Nullable != new.Nullable || c.Type != new.Type {
		colType := new.Type
		if new.Nullable {
			colType = fmt.Sprintf("Nullable(%s)", new.Type)
		}
		queries = append(queries, fmt.Sprintf("MODIFY COLUMN %s %s", c.Name, colType))
	}

	// Default
	{
		if new.Default != nil && (c.Default == nil || *c.Default != *new.Default) {
			queries = append(queries, fmt.Sprintf("MODIFY COLUMN %s DEFAULT %s", c.Name, *new.Default))
		} else if c.Default != nil && new.Default == nil {
			queries = append(queries, fmt.Sprintf("MODIFY COLUMN %s REMOVE DEFAULT", c.Name))
		}
	}

	// Materialized
	{
		if new.Materialized != nil && (c.Materialized == nil || *c.Materialized != *new.Materialized) {
			queries = append(queries, fmt.Sprintf("MODIFY COLUMN %s MATERIALIZED %s", c.Name, *new.Materialized))
		} else if c.Materialized != nil && new.Materialized == nil {
			queries = append(queries, fmt.Sprintf("MODIFY COLUMN %s REMOVE MATERIALIZED", c.Name))
		}
	}

	// Alias
	{
		if new.Alias != nil && (c.Alias == nil || *c.Alias != *new.Alias) {
			queries = append(queries, fmt.Sprintf("MODIFY COLUMN %s ALIAS %s", c.Name, *new.Alias))
		} else if c.Alias != nil && new.Alias == nil {
			queries = append(queries, fmt.Sprintf("MODIFY COLUMN %s REMOVE ALIAS", c.Name))
		}
	}

	// Comment
	{
		if new.Comment != nil && (c.Comment == nil || *c.Comment != *new.Comment) {
			queries = append(queries, fmt.Sprintf("MODIFY COLUMN %s COMMENT '%s'", c.Name, *new.Comment))
		} else if c.Comment != nil && new.Comment == nil {
			queries = append(queries, fmt.Sprintf("MODIFY COLUMN %s REMOVE COMMENT", c.Name))
		}
	}

	return queries
}
