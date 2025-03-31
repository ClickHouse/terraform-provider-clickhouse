package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/huandu/go-sqlbuilder"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/project"
)

type ResponseWithResult[T any] struct {
	Result T `json:"result"`
}

func (c *ClientImpl) getOrgPath(path string) string {
	return fmt.Sprintf("%s/organizations/%s%s", c.BaseUrl, c.OrganizationId, path)
}

func (c *ClientImpl) getServicePath(serviceId string, path string) string {
	if serviceId == "" {
		return c.getOrgPath("/services")
	}
	return c.getOrgPath(fmt.Sprintf("/services/%s%s", serviceId, path))
}

func (c *ClientImpl) getPrivateEndpointConfigPath(cloudProvider string, region string) string {
	return c.getOrgPath(fmt.Sprintf("/privateEndpointConfig?cloud_provider=%s&region_id=%s", cloudProvider, region))
}

func (c *ClientImpl) getQueryAPIPath(queryAPIBaseUrl string, serviceID string, format string) string { //nolint
	if format == "" {
		panic("format can't be empty in getQueryAPIPath")
	}
	return fmt.Sprintf("%s/.api/services/%s/query?format=%s", queryAPIBaseUrl, serviceID, format)
}

func (c *ClientImpl) doRequest(ctx context.Context, initialReq *http.Request) ([]byte, error) {
	ctx = tflog.SetField(ctx, "method", initialReq.Method)
	ctx = tflog.SetField(ctx, "URL", initialReq.URL.String())

	initialReq.SetBasicAuth(c.TokenKey, c.TokenSecret)

	currentExponentialBackoff := float64(4)
	attempt := 1

	// Copy the request body as a tflog field to have it logged.
	var bodyBytes []byte
	if initialReq.Body != nil {
		bodyBytes, _ = io.ReadAll(initialReq.Body)
		initialReq.Body.Close()
		initialReq.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		{
			var buf bytes.Buffer
			err := json.Indent(&buf, bodyBytes, "", "  ")
			if err != nil {
				// Parsing/indentation failed, fallback to raw body
				ctx = tflog.SetField(ctx, "requestBody", string(bodyBytes))
			} else {
				// Parsing ok, use formatted body.
				ctx = tflog.SetField(ctx, "requestBody", buf.String())
			}
		}

		initialReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	}

	initialReq.Header.Set("User-Agent", fmt.Sprintf("terraform-provider-clickhouse/%s Commit/%s", project.Version(), project.Commit()))

	{
		// Redact sensitive headers from logs.
		headers := initialReq.Header.Clone()
		headers.Set("Authorization", "Basic REDACTED")
		ctx = tflog.SetField(ctx, "requestHeaders", headers)
	}

	makeRequest := func() ([]byte, error) {
		req := initialReq.Clone(ctx)
		// Set the body again to make the stream go back to the beginning.
		if bodyBytes != nil {
			req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
		ctx = tflog.SetField(ctx, "attempt", attempt)
		attempt = attempt + 1

		start := time.Now()
		ctx = tflog.SetField(ctx, "requestStartedAt", start.Format(time.RFC3339Nano))

		res, err := c.HttpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}

		stop := time.Now()

		ctx = tflog.SetField(ctx, "responseReceivedAt", stop.Format(time.RFC3339Nano))
		ctx = tflog.SetField(ctx, "requestTimeMS", stop.Sub(start).Milliseconds())
		ctx = tflog.SetField(ctx, "statusCode", res.StatusCode)
		ctx = tflog.SetField(ctx, "responseHeaders", res.Header)
		{
			var buf bytes.Buffer
			err = json.Indent(&buf, body, "", "  ")
			if err != nil {
				// Parsing/indentation failed, fallback to raw body
				ctx = tflog.SetField(ctx, "responseBody", string(body))
			} else {
				// Parsing ok, use formatted body.
				ctx = tflog.SetField(ctx, "responseBody", buf.String())
			}
		}
		tflog.Debug(ctx, "API request")

		if res.StatusCode != http.StatusOK {
			var resetSeconds float64
			if res.StatusCode == http.StatusTooManyRequests { // 429
				// Try to read rate limiting headers from the response.
				resetSecondsStr := res.Header.Get(ResponseHeaderRateLimitReset)
				if resetSecondsStr != "" {
					// Try parsing the string as an integer
					i, err := strconv.ParseFloat(resetSecondsStr, 64)
					if err != nil {
						tflog.Warn(ctx, fmt.Sprintf("Error parsing X-RateLimit-Reset header %q as a float64: %s", resetSecondsStr, err))
					} else {
						// Give 1 more second after the server returned reset.
						resetSeconds = i + 1

						tflog.Warn(ctx, fmt.Sprintf("Server side throttling (429): waiting %f.1 seconds before retrying", resetSeconds))
					}
				}
			} else if res.StatusCode >= http.StatusInternalServerError { // 500
				resetSeconds = currentExponentialBackoff
				tflog.Warn(ctx, fmt.Sprintf("Server side error (5xx): waiting %f.1 seconds before retrying", resetSeconds))
			} else {
				return nil, backoff.Permanent(fmt.Errorf("status: %d, body: %s", res.StatusCode, body))
			}

			// Wait for the calculated exponential backoff number of seconds.
			time.Sleep(time.Second * time.Duration(resetSeconds))

			// Double wait time for next loop
			currentExponentialBackoff = currentExponentialBackoff * 2

			return nil, fmt.Errorf("status: %d, body: %s", res.StatusCode, body)
		}

		return body, nil
	}

	// This is a fake exponential backoff, because multiplier is only 1.
	// We need to do this because there is no way to set a MaxElapsedTime using ConstantBackOff()
	// Real waiting times happen in the makeRequest function depending on the server's response.
	backoffSettings := backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(1*time.Second),
		backoff.WithMaxElapsedTime(61*time.Second),
		backoff.WithMultiplier(1),
	)

	body, err := backoff.RetryWithData[[]byte](makeRequest, backoffSettings)

	return body, err
}

func (c *ClientImpl) runQuery(ctx context.Context, serviceID string, sql string, args ...interface{}) ([]byte, error) { //nolint
	return c.runQueryWithFormat(ctx, serviceID, "JSONEachRow", sql, args...)
}

func (c *ClientImpl) runQueryWithFormat(ctx context.Context, serviceID string, format string, sql string, args ...interface{}) ([]byte, error) { //nolint
	// TODO once openAPI will expose this information, make a call to get it dynamically.
	queryAPIBaseUrl := "https://console-api.clickhouse.cloud"

	qry, err := sqlbuilder.ClickHouse.Interpolate(sql, args)
	if err != nil {
		return nil, err
	}

	s := struct {
		SQL string `json:"sql"`
	}{
		SQL: qry,
	}

	buffer := &bytes.Buffer{}
	if err := json.NewEncoder(buffer).Encode(&s); err != nil {
		return nil, fmt.Errorf("encoding query payload to JSON failed: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.getQueryAPIPath(queryAPIBaseUrl, serviceID, format), buffer)
	if err != nil {
		return nil, err
	}

	return c.doRequest(ctx, req)
}
