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

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/project"
)

type ResponseWithResult[T any] struct {
	Result T `json:"result"`
}

const (
	redactedPlaceholder            = "REDACTED"
	unparseableRedactedPlaceholder = `"<unparseable, redacted>"`
)

var sensitiveBodyKeys = map[string]struct{}{
	"password":          {},
	"newPassword":       {},
	"newPasswordHash":   {},
	"newDoubleSha1Hash": {},
	"password_wo":       {},
	"tokenSecret":       {},
	// Postgres connection strings embed the generated password in the URI.
	"connectionString":  {},
	"connection_string": {},
}

var sensitiveBodyContainers = map[string]struct{}{
	"secrets":     {},
	"credentials": {},
}

// redactSensitiveBody returns body with values of known sensitive keys replaced
// by a placeholder string. Walks JSON recursively; arrays and nested objects
// are traversed. Containers named "secrets" or "credentials" have their entire
// subtree replaced by a scalar placeholder.
//
// Empty input is returned unchanged. Malformed JSON returns a generic placeholder
// rather than the raw bytes so a logging path never leaks unredacted content.
func redactSensitiveBody(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return []byte(unparseableRedactedPlaceholder)
	}
	redacted := redactJSONValue(v)
	out, err := json.Marshal(redacted)
	if err != nil {
		return []byte(unparseableRedactedPlaceholder)
	}
	return out
}

// formatLogBody returns a string suitable for logging the given JSON body.
// Sensitive fields are redacted and the output is pretty-printed when possible.
// Empty input yields an empty string. Malformed JSON yields a generic placeholder
// rather than leaking the raw bytes into logs.
func formatLogBody(ctx context.Context, body []byte) string {
	if len(body) == 0 {
		return ""
	}
	redacted := redactSensitiveBody(body)
	var buf bytes.Buffer
	if err := json.Indent(&buf, redacted, "", "  "); err != nil {
		// Should be unreachable: redactSensitiveBody emits valid JSON for any
		// non-empty input. Surface the failure rather than silently returning
		// the placeholder.
		tflog.Warn(ctx, "formatLogBody: json.Indent failed on already-redacted output", map[string]any{"error": err.Error()})
		return unparseableRedactedPlaceholder
	}
	return buf.String()
}

func redactJSONValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if _, ok := sensitiveBodyKeys[k]; ok {
				t[k] = redactedPlaceholder
				continue
			}
			if _, ok := sensitiveBodyContainers[k]; ok {
				t[k] = redactedPlaceholder
				continue
			}
			t[k] = redactJSONValue(child)
		}
		return t
	case []any:
		for i, child := range t {
			t[i] = redactJSONValue(child)
		}
		return t
	default:
		return v
	}
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

func (c *ClientImpl) getPostgresPath(postgresId string, path string) string {
	if postgresId == "" {
		return c.getOrgPath("/postgres")
	}
	return c.getOrgPath(fmt.Sprintf("/postgres/%s%s", postgresId, path))
}

func (c *ClientImpl) getQueryAPIPath(queryAPIBaseUrl string, serviceID string, format string) string { //nolint
	if format == "" {
		panic("format can't be empty in getQueryAPIPath")
	}
	return fmt.Sprintf("%s/.api/services/%s/query?format=%s", queryAPIBaseUrl, serviceID, format)
}

func (c *ClientImpl) doRequest(ctx context.Context, initialReq *http.Request) ([]byte, error) {
	debugctx := tflog.SetField(ctx, "request", fmt.Sprintf("%s %s", initialReq.Method, initialReq.URL.String()))
	debugctx = tflog.SetField(debugctx, "clientTimeout", c.HttpClient.Timeout.String())

	initialReq.SetBasicAuth(c.TokenKey, c.TokenSecret)

	currentExponentialBackoff := float64(4)
	attempt := 1

	// Copy the request body as a tflog field to have it logged.
	var bodyBytes []byte
	if initialReq.Body != nil {
		bodyBytes, _ = io.ReadAll(initialReq.Body)
		initialReq.Body.Close()
		initialReq.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		debugctx = tflog.SetField(debugctx, "requestBody", formatLogBody(debugctx, bodyBytes))

		initialReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	}

	initialReq.Header.Set("User-Agent", fmt.Sprintf("terraform-provider-clickhouse/%s Commit/%s", project.Version(), project.Commit()))

	{
		// Redact sensitive headers from logs.
		headers := initialReq.Header.Clone()
		headers.Set("Authorization", "Basic REDACTED")
		debugctx = tflog.SetField(debugctx, "requestHeaders", headers)
	}

	makeRequest := func() ([]byte, error) {
		req := initialReq.Clone(ctx)
		// Set the body again to make the stream go back to the beginning.
		if bodyBytes != nil {
			req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
		debugctx = tflog.SetField(debugctx, "attempt", attempt)
		attempt = attempt + 1

		start := time.Now()
		debugctx = tflog.SetField(debugctx, "requestStartedAt", start.Format(time.RFC3339Nano))

		res, err := c.HttpClient.Do(req)
		if err != nil {
			debugctx = tflog.SetField(debugctx, "error", err.Error())
			tflog.Debug(debugctx, "API request failed")
			return nil, err
		}
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}

		stop := time.Now()

		debugctx = tflog.SetField(debugctx, "responseReceivedAt", stop.Format(time.RFC3339Nano))
		debugctx = tflog.SetField(debugctx, "requestTimeMS", stop.Sub(start).Milliseconds())
		debugctx = tflog.SetField(debugctx, "statusCode", res.StatusCode)
		debugctx = tflog.SetField(debugctx, "responseHeaders", res.Header)
		debugctx = tflog.SetField(debugctx, "responseBody", formatLogBody(debugctx, body))
		tflog.Debug(debugctx, "API request")

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
	// backoff.WithContext makes a cancelled ctx (Ctrl-C, Terraform deadline)
	// abort the inner retry loop instead of waiting out the 61s MaxElapsedTime
	// — so cancellation propagation from outer wait helpers is prompt.
	innerBackoff := backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(1*time.Second),
		backoff.WithMaxElapsedTime(61*time.Second),
		backoff.WithMultiplier(1),
	)
	backoffSettings := backoff.WithContext(innerBackoff, ctx)

	body, err := backoff.RetryWithData[[]byte](makeRequest, backoffSettings)

	return body, err
}
