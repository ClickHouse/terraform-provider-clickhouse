package tableBuilder

import (
	"errors"
	"regexp"
	"strings"
)

func parseEngineFull(engineFull string) (*Engine, map[string]string, error) {
	// CollapsingMergeTree(sign) ORDER BY id SETTINGS index_granularity = 1024, test = true

	r := regexp.MustCompile(`^(?P<EngineName>[a-zA-Z]+)[(](?P<Params>.*)[)].*SETTINGS (?P<Settings>.*)$`)

	if !r.Match([]byte(engineFull)) {
		return nil, nil, errors.New("cannot parse engine_full field")
	}

	matches := r.FindStringSubmatch(engineFull)

	var params []string
	{
		// "sign, other"
		paramsString := matches[r.SubexpIndex("Params")]

		dirtyParams := strings.Split(paramsString, ",")
		for _, p := range dirtyParams {
			params = append(params, strings.TrimSpace(p))
		}
	}
	settings := make(map[string]string)
	{
		// "index_granularity = 1024, test = true"
		settingsString := matches[r.SubexpIndex("Settings")]

		rawSettingsList := strings.Split(settingsString, ",")

		for _, s := range rawSettingsList {
			// "index_granularity = 1024"
			splitted := strings.Split(s, "=")

			settings[strings.TrimSpace(splitted[0])] = strings.TrimSpace(splitted[1])
		}

	}

	engine := &Engine{
		Name:   matches[r.SubexpIndex("EngineName")],
		Params: params,
	}

	return engine, settings, nil
}
