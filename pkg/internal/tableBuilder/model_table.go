package tableBuilder

import (
	"fmt"
	"strings"
)

type Table struct {
	Name    string
	Columns []Column
	OrderBy string
}

func (t *Table) querySpec() string {
	var columns []string
	for _, c := range t.Columns {
		columns = append(columns, c.querySpec())
	}
	return fmt.Sprintf("CREATE TABLE %s (%s) ORDER BY %s;", t.Name, strings.Join(columns, ", "), t.OrderBy)
}
