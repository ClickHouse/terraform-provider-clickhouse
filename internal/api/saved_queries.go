package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type SavedQueryRequest struct {
	Name       string            `json:"name"`
	SQL        string            `json:"sql"`
	Database   string            `json:"database"`
	Parameters map[string]string `json:"parameters"`
}

type SavedQuery struct {
	SavedQueryRequest
	ID string `json:"id"`
}

func (c *ClientImpl) savedQueriesPath(serviceID, queryID string) string {
	path := c.getServicePath(serviceID, "/saved-queries")
	if queryID != "" {
		path += "/" + queryID
	}
	return path
}

func (c *ClientImpl) CreateSavedQuery(
	ctx context.Context,
	serviceID string,
	query SavedQueryRequest,
) (*SavedQuery, error) {
	requestBody, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("encode create saved query request: %w", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		c.savedQueriesPath(serviceID, ""),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, fmt.Errorf("create saved query request: %w", err)
	}

	body, err := c.doRequestWithAcceptedStatus(ctx, req, http.StatusCreated)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[SavedQuery]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode created saved query: %w", err)
	}
	return &response.Result, nil
}

func (c *ClientImpl) GetSavedQuery(
	ctx context.Context,
	serviceID string,
	queryID string,
) (*SavedQuery, error) {
	req, err := http.NewRequest(
		http.MethodGet,
		c.savedQueriesPath(serviceID, queryID),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create get saved query request: %w", err)
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[SavedQuery]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode saved query %q: %w", queryID, err)
	}
	return &response.Result, nil
}

func (c *ClientImpl) UpdateSavedQuery(
	ctx context.Context,
	serviceID string,
	queryID string,
	query SavedQueryRequest,
) (*SavedQuery, error) {
	requestBody, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("encode update saved query request: %w", err)
	}

	req, err := http.NewRequest(
		http.MethodPut,
		c.savedQueriesPath(serviceID, queryID),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, fmt.Errorf("create update saved query request: %w", err)
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[SavedQuery]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode updated saved query %q: %w", queryID, err)
	}
	return &response.Result, nil
}

func (c *ClientImpl) DeleteSavedQuery(
	ctx context.Context,
	serviceID string,
	queryID string,
) error {
	req, err := http.NewRequest(
		http.MethodDelete,
		c.savedQueriesPath(serviceID, queryID),
		nil,
	)
	if err != nil {
		return fmt.Errorf("create delete saved query request: %w", err)
	}

	_, err = c.doRequest(ctx, req)
	return err
}
