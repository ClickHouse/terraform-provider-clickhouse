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

	var engine string
	if t.Engine.Name != "" {
		engine = fmt.Sprintf(" Engine=%s(%s)", t.Engine.Name, strings.Join(t.Engine.Params, ", "))
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

	return fmt.Sprintf("CREATE OR REPLACE TABLE %s (%s)%s ORDER BY %s%s%s;", t.Name, strings.Join(columns, ", "), engine, t.OrderBy, settings, comment)
}

func (t *Table) diffQueries(new Table) []string {
	queries := make([]string, 0)

	// Comment
	if t.Comment != new.Comment {
		queries = append(queries, fmt.Sprintf("ALTER TABLE %s MODIFY COMMENT '%s';", t.Name, new.Comment))
	}

	// Settings
	{
		for name, current := range t.Settings {
			desired, found := new.Settings[name]
			if found {
				if current != desired {
					// Setting value changed.
					queries = append(queries, fmt.Sprintf("ALTER TABLE %s MODIFY SETTING %s=%s;", t.Name, name, desired))
				}
			} else {
				// Setting was removed from tf file
				queries = append(queries, fmt.Sprintf("ALTER TABLE %s RESET SETTING %s;", t.Name, name))
			}
		}
		for name, desired := range new.Settings {
			_, found := t.Settings[name]
			if !found {
				// Setting was added.
				queries = append(queries, fmt.Sprintf("ALTER TABLE %s MODIFY SETTING %s=%s;", t.Name, name, desired))
			}
		}
	}

	// Columns
	{
		oldColumns := make(map[string]Column)
		newColumns := make(map[string]Column)

		for _, oldCol := range t.Columns {
			oldColumns[oldCol.Name] = oldCol
		}
		for _, newCol := range new.Columns {
			newColumns[newCol.Name] = newCol
		}

		for name, existing := range oldColumns {
			desired, found := newColumns[name]
			if found {
				if existing != desired {
					// Column spec was changed
					for _, q := range existing.diffQueries(desired) {
						queries = append(queries, fmt.Sprintf("ALTER TABLE %s %s;", t.Name, q))
					}
				}
			} else {
				// Column was removed from tf file
				queries = append(queries, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", t.Name, name))
			}
		}
		for name, desired := range newColumns {
			_, found := oldColumns[name]
			if !found {
				// Column was added in tf file.
				queries = append(queries, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", t.Name, desired.querySpec()))
			}
		}
	}

	return queries
}
