package tableBuilder

import (
	"fmt"
	"strings"
)

type Table struct {
	Name     string
	Engine   Engine
	Columns  []Column
	OrderBy  string
	Settings map[string]string
	Comment  string
}

type Engine struct {
	Name   string
	Params []string
}

func (t *Table) querySpec() string {
	var columns []string
	for _, c := range t.Columns {
		columns = append(columns, c.querySpec())
	}

	settingsList := make([]string, 0)
	for name, value := range t.Settings {
		settingsList = append(settingsList, fmt.Sprintf("%s=%s", name, value))
	}

	var settings string
	if len(settingsList) > 0 {
		settings = fmt.Sprintf(" SETTINGS %s", strings.Join(settingsList, ", "))
	}

	var comment string
	if len(t.Comment) > 0 {
		comment = fmt.Sprintf(" COMMENT '%s'", t.Comment)
	}

	return fmt.Sprintf("CREATE OR REPLACE TABLE %s (%s) Engine=%s(%s) ORDER BY %s%s%s;", t.Name, strings.Join(columns, ", "), t.Engine.Name, strings.Join(t.Engine.Params, ", "), t.OrderBy, settings, comment)
}
