package tableBuilder

import (
	"fmt"
	"strings"
)

type Table struct {
	Name    string
	Columns []Column
	OrderBy string
	Comment string
}

func (t *Table) querySpec() string {
	var columns []string
	for _, c := range t.Columns {
		columns = append(columns, c.querySpec())
	}
	return fmt.Sprintf("CREATE OR REPLACE TABLE %s (%s) ORDER BY %s COMMENT '%s';", t.Name, strings.Join(columns, ", "), t.OrderBy, t.Comment)
}
