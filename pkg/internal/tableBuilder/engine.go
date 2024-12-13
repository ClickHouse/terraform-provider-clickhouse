package tableBuilder

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

func parseEngineFull(engineFull string) (*Engine, map[string]string, error) {
	// CollapsingMergeTree(sign) ORDER BY id SETTINGS index_granularity = 1024, test = true

	// Parse Engine and params
	var engineName string
	var params []string
	{
		i := strings.Index(engineFull, " ORDER BY")
		if i < 0 {
			return nil, nil, errors.New("Didn't find expected ' ORDER BY' substring in engine_full field")
		}

		engine := engineFull[0:i]

		r := regexp.MustCompile(`^(?P<EngineName>[a-zA-Z]+)[(]?(?P<Params>[^)]*)[)]?$`)
		if !r.Match([]byte(engine)) {
			return nil, nil, errors.New("cannot parse engine_full field")
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
					return nil, nil, errors.New(fmt.Sprintf("cannot parse settings: expected exactly one = sign for each setting, got %d", len(splitted)))
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
