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
	KeySuffix string `json:"keySuffix"`
}

func (c *ClientImpl) GetApiKeyID(ctx context.Context) (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.getOrgPath("/keys"), nil)
	if err != nil {
		return "", err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return "", err
	}

	apiKeysResponse := ResponseWithResult[[]ApiKey]{}
	err = json.Unmarshal(body, &apiKeysResponse)
	if err != nil {
		return "", err
	}

	for _, key := range apiKeysResponse.Result {
		// This is a very weak check, but that's all we have until https://github.com/ClickHouse/control-plane/issues/13294 is implemented.
		if strings.HasSuffix(c.TokenKey, key.KeySuffix) {
			return key.ID, nil
		}
	}

	return "", fmt.Errorf("key not found")
}
