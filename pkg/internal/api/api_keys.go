package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type ApiKey struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	KeySuffix string `json:"keySuffix"`
}

func (c *ClientImpl) GetApiKeyID(ctx context.Context, name *string) (*ApiKey, error) {
	req, err := http.NewRequest(http.MethodGet, c.getOrgPath("/keys"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	apiKeysResponse := ResponseWithResult[[]ApiKey]{}
	err = json.Unmarshal(body, &apiKeysResponse)
	if err != nil {
		return nil, err
	}

	for _, key := range apiKeysResponse.Result {
		// We want to match a key by name
		if name != nil {
			if *name == key.Name {
				return &key, nil
			}
		} else {
			// Find id of the API key configured for this terraform provider run.
			// Since we don't know the ID of the API key on the client side, we need to a pattern match on the KeySuffix.
			// This is a very weak check, but that's all we have until https://github.com/ClickHouse/control-plane/issues/13294 is implemented.
			if strings.HasSuffix(c.TokenKey, key.KeySuffix) {
				return &key, nil
			}
		}
	}

	errorMsg := "key not found"

	if name != nil {
		errorMsg = fmt.Sprintf("API key named %q was not found", *name)
	}

	return nil, fmt.Errorf(errorMsg) //nolint
}
