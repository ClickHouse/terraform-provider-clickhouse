package api

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/huandu/go-sqlbuilder"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/sql"
)

type Table struct {
	Database string `json:"database"`
	Name     string `json:"name"`
	Engine   Engine
	Columns  []Column
	OrderBy  string `json:"primary_key"`
	Settings map[string]string
	Comment  string `json:"comment"`

	EngineFull string `json:"engine_full"`
}

type Engine struct {
	Name   string
	Params []string
}

type Column struct {
	Name         string `json:"name"`
	Type         string
	Nullable     bool
	Default      *string
	Materialized *string
	Ephemeral    bool
	Alias        *string
	Comment      *string

	TypeWithNullable  string `json:"type"`
	DefaultType       string `json:"default_type"`
	DefaultExpression string `json:"default_expression"`
}

func (c *ClientImpl) CreateTable(ctx context.Context, serviceID string, table Table) error {
	builder := sqlbuilder.CreateTable(fmt.Sprintf("`%s`.`%s`", sql.EscapeBacktick(table.Database), sql.EscapeBacktick(table.Name)))
	options := make([]string, 0)

	for _, col := range table.Columns {
		definitions := []string{
			col.Name,
		}
		if col.Nullable {
			definitions = append(definitions, fmt.Sprintf("Nullable(%s)", col.Type))
		} else {
			definitions = append(definitions, col.Type)
		}
		if col.Default != nil {
			definitions = append(definitions, "DEFAULT")
			definitions = append(definitions, *col.Default)
		}
		if col.Materialized != nil {
			definitions = append(definitions, "MATERIALIZED")
			definitions = append(definitions, *col.Materialized)
		}
		if col.Alias != nil {
			definitions = append(definitions, "ALIAS")
			definitions = append(definitions, *col.Alias)
		}
		if col.Comment != nil {
			definitions = append(definitions, "COMMENT")
			definitions = append(definitions, fmt.Sprintf("'%s'", sql.EscapeSingleQuote(*col.Comment)))
		}
		if col.Ephemeral {
			definitions = append(definitions, "EPHEMERAL")
		}
		builder.Define(definitions...)
	}

	if table.Engine.Name != "" {
		options = append(options, fmt.Sprintf("Engine=%s(%s)", table.Engine.Name, strings.Join(table.Engine.Params, ", ")))
	}

	options = append(options, "ORDER BY", table.OrderBy)

	// SETTINGS
	{
		settingsList := make([]string, 0)
		for name, value := range table.Settings {
			settingsList = append(settingsList, fmt.Sprintf("%s=%s", name, value))
		}

		if len(settingsList) > 0 {
			options = append(options, "SETTINGS", strings.Join(settingsList, ", "))
		}
	}

	// COMMENT
	if len(table.Comment) > 0 {
		options = append(options, "COMMENT", table.Comment)
	}

	builder.Option(options...)

	qry, args := builder.BuildWithFlavor(sqlbuilder.ClickHouse)

	_, err := c.runQuery(ctx, serviceID, qry, args...)
	if err != nil {
		return err
	}

	return nil
}

func (c *ClientImpl) GetTable(ctx context.Context, serviceID, database, name string) (*Table, error) {
	table := Table{}

	// Main table fields
	{
		builder := sqlbuilder.NewSelectBuilder()
		builder.Select("database", "name", "engine_full", "primary_key", "comment")
		builder.From("system.tables")
		builder.Where(builder.Equal("database", database), builder.Equal("name", name))

		qry, args := builder.BuildWithFlavor(sqlbuilder.ClickHouse)

		body, err := c.runQuery(ctx, serviceID, qry, args...)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal(body, &table)
		if err != nil {
			return nil, err
		}
	}

	// Columns
	{
		qry, args := sqlbuilder.Build("DESCRIBE TABLE `$?`.`$?`;", sqlbuilder.Raw(sql.EscapeBacktick(database)), sqlbuilder.Raw(sql.EscapeBacktick(name))).BuildWithFlavor(sqlbuilder.ClickHouse)
		body, err := c.runQueryWithFormat(ctx, serviceID, "JSON", qry, args...)
		if err != nil {
			return nil, err
		}

		resp := struct {
			Columns []Column `json:"data"`
		}{}
		err = json.Unmarshal(body, &resp)
		if err != nil {
			return nil, err
		}

		for _, col := range resp.Columns {
			if strings.HasPrefix(col.TypeWithNullable, "Nullable") {
				col.Nullable = true
				col.Type = strings.TrimSuffix(strings.TrimPrefix(col.TypeWithNullable, "Nullable("), ")")
			} else {
				col.Type = col.TypeWithNullable
			}

			switch col.DefaultType {
			case "ALIAS":
				col.Alias = &col.DefaultExpression
			case "DEFAULT":
				col.Default = &col.DefaultExpression
			case "EPHEMERAL":
				col.Ephemeral = true
			case "MATERIALIZED":
				col.Materialized = &col.DefaultExpression
			}

			table.Columns = append(table.Columns, col)
		}
	}

	// Settings and Engine
	{
		engine, settings, err := parseEngineFull(table.EngineFull)
		if err != nil {
			return nil, err
		}

		table.Engine = *engine
		table.Settings = settings
	}

	return &table, nil
}

func (c *ClientImpl) DeleteTable(ctx context.Context, serviceID, database, name string) error {
	sb := sqlbuilder.Build("DROP TABLE IF EXISTS `$?`.`$?`;", sqlbuilder.Raw(sql.EscapeBacktick(database)), sqlbuilder.Raw(sql.EscapeBacktick(name)))
	qry, args := sb.Build()
	_, err := c.runQuery(ctx, serviceID, qry, args...)
	if err != nil {
		return err
	}

	return nil
}

func parseEngineFull(engineFull string) (*Engine, map[string]string, error) {
	// CollapsingMergeTree(sign) ORDER BY id SETTINGS index_granularity = 1024, test = true

	// Parse Engine and params
	var engineName string
	var params []string
	{
		i := strings.Index(engineFull, " ORDER BY")
		if i < 0 {
			return nil, nil, fmt.Errorf("didn't find expected ' ORDER BY' substring in engine_full field %q", engineFull)
		}

		engine := engineFull[0:i]

		r := regexp.MustCompile(`^(?P<EngineName>[a-zA-Z]+)[(]?(?P<Params>[^)]*)[)]?$`)
		if !r.Match([]byte(engine)) {
			return nil, nil, fmt.Errorf("cannot parse engine_full field")
		}

		matches := r.FindStringSubmatch(engine)

		engineName = matches[r.SubexpIndex("EngineName")]

		if r.SubexpIndex("Params") > 0 && matches[r.SubexpIndex("Params")] != "" {
			// "sign, other"
			paramsString := matches[r.SubexpIndex("Params")]

			dirtyParams := strings.Split(paramsString, ",")
			for _, p := range dirtyParams {
				params = append(params, strings.TrimSpace(p))
			}
		}
	}

	var settings map[string]string
	{
		i := strings.Index(engineFull, "SETTINGS ")
		if i > 0 {
			settings = make(map[string]string)
			rawSettingsList := strings.Split(engineFull[i+9:], ",")
			for _, s := range rawSettingsList {
				// "index_granularity = 1024"

				splitted := strings.Split(s, "=")

				if len(splitted) != 2 {
					return nil, nil, fmt.Errorf("cannot parse settings: expected exactly one = sign for each setting, got %d", len(splitted))
				}

				settings[strings.TrimSpace(splitted[0])] = strings.TrimSpace(splitted[1])
			}
		}
	}

	engine := &Engine{
		Name:   engineName,
		Params: params,
	}

	return engine, settings, nil
}
