package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
)

func (c *ClientImpl) getPostgresPath(postgresId string, subpath string) string {
	if postgresId == "" {
		return c.getOrgPath("/postgres")
	}
	return c.getOrgPath(fmt.Sprintf("/postgres/%s%s", postgresId, subpath))
}

// GetPostgresInstance returns a Postgres instance by ID.
func (c *ClientImpl) GetPostgresInstance(ctx context.Context, postgresId string) (*PostgresInstance, error) {
	req, err := http.NewRequest(http.MethodGet, c.getPostgresPath(postgresId, ""), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[PostgresInstance]{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}

	return &response.Result, nil
}

// CreatePostgresInstance creates a new Postgres instance.
func (c *ClientImpl) CreatePostgresInstance(ctx context.Context, instance PostgresInstanceCreate) (*PostgresInstance, error) {
	rb, err := json.Marshal(instance)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, c.getPostgresPath("", ""), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[PostgresInstance]{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}

	return &response.Result, nil
}

// UpdatePostgresInstance updates an existing Postgres instance.
func (c *ClientImpl) UpdatePostgresInstance(ctx context.Context, postgresId string, update PostgresInstanceUpdate) (*PostgresInstance, error) {
	rb, err := json.Marshal(update)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPatch, c.getPostgresPath(postgresId, ""), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[PostgresInstance]{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}

	return &response.Result, nil
}

// DeletePostgresInstance deletes a Postgres instance by ID.
func (c *ClientImpl) DeletePostgresInstance(ctx context.Context, postgresId string) error {
	req, err := http.NewRequest(http.MethodDelete, c.getPostgresPath(postgresId, ""), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(ctx, req)
	return err
}

// GetPostgresInstanceCACertificate returns the CA certificate PEM for a Postgres instance.
func (c *ClientImpl) GetPostgresInstanceCACertificate(ctx context.Context, postgresId string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.getPostgresPath(postgresId, "/caCertificates"), nil)
	if err != nil {
		return "", err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// WaitForPostgresInstanceState polls until the Postgres instance reaches a desired state.
func (c *ClientImpl) WaitForPostgresInstanceState(ctx context.Context, postgresId string, stateChecker func(string) bool, maxWaitSeconds int) error {
	checkState := func() error {
		instance, err := c.GetPostgresInstance(ctx, postgresId)
		if is5xx(err) {
			// 500s are automatically retried in `GetPostgresInstance`.
			// If we get it here, we consider it an unrecoverable error.
			return backoff.Permanent(err)
		} else if err != nil {
			return err
		}

		if stateChecker(instance.State) {
			return nil
		}

		return fmt.Errorf("postgres instance %s is in state %s", postgresId, instance.State)
	}

	if maxWaitSeconds < 5 {
		maxWaitSeconds = 5
	}

	err := backoff.Retry(checkState, backoff.WithMaxRetries(backoff.NewConstantBackOff(5*time.Second), uint64(maxWaitSeconds/5))) //nolint:gosec
	if err != nil {
		return err
	}

	return nil
}
