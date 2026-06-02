package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
)

// Sentinel errors distinguishing "poll succeeded but target not yet met"
// (legitimate retry/timeout) from real GetPostgres errors (must propagate).
var (
	errPostgresStateUnchanged    = errors.New("postgres state unchanged from terminal")
	errPostgresStateNotYetTarget = errors.New("postgres state checker not yet true")
)

// Retry budgets. Overridable per-call via the *WithBudget / *WithInterval
// helpers so unit tests can run in milliseconds.
var (
	postgresDeleteRetryInterval        = 10 * time.Second
	postgresDeleteRetryAttempts uint64 = 90 // 90 × 10s = 15 minutes
	postgresStatePollInterval          = 5 * time.Second
	// Predicate must hold this long before WaitForPostgresMatch succeeds —
	// rejects transient matches during multi-step transitions. 15s = 3 polls.
	postgresStateSettleDuration = 15 * time.Second
)

// ---------------------------------------------------------------------------
// GET / LIST
// ---------------------------------------------------------------------------

// GetPostgres fetches a single Postgres instance by ID. A 404 response yields
// an error for which IsNotFound returns true.
func (c *ClientImpl) GetPostgres(ctx context.Context, postgresId string) (*Postgres, error) {
	req, err := http.NewRequest(http.MethodGet, c.getPostgresPath(postgresId, ""), nil)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	resp := ResponseWithResult[Postgres]{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Postgres: %w", err)
	}
	return &resp.Result, nil
}

// ListPostgres returns every Postgres instance the org has access to.
// Used by future data sources (Phase 5).
func (c *ClientImpl) ListPostgres(ctx context.Context) ([]PostgresListItem, error) {
	req, err := http.NewRequest(http.MethodGet, c.getPostgresPath("", ""), nil)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	resp := ResponseWithResult[[]PostgresListItem]{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Postgres list: %w", err)
	}
	return resp.Result, nil
}

// ---------------------------------------------------------------------------
// CREATE / UPDATE / DELETE
// ---------------------------------------------------------------------------

// CreatePostgres provisions a new instance. Returns the instance plus the
// server-generated password as a separate return value (non-empty only when
// the request body had no password set). Callers MUST persist the password
// from this response — subsequent GETs are not guaranteed to echo it back.
func (c *ClientImpl) CreatePostgres(ctx context.Context, body PostgresCreate) (*Postgres, string, error) {
	rb, err := json.Marshal(body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to encode PostgresCreate: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.getPostgresPath("", ""), bytes.NewReader(rb))
	if err != nil {
		return nil, "", err
	}
	respBody, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, "", err
	}
	resp := ResponseWithResult[Postgres]{}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal Postgres: %w", err)
	}
	return &resp.Result, resp.Result.Password, nil
}

// UpdatePostgres PATCHes size / haType / tags. Other fields would be rejected
// by the server; PostgresUpdate's shape enforces this.
func (c *ClientImpl) UpdatePostgres(ctx context.Context, postgresId string, body PostgresUpdate) (*Postgres, error) {
	rb, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to encode PostgresUpdate: %w", err)
	}
	req, err := http.NewRequest(http.MethodPatch, c.getPostgresPath(postgresId, ""), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}
	respBody, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	resp := ResponseWithResult[Postgres]{}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Postgres: %w", err)
	}
	return &resp.Result, nil
}

// DeletePostgres deletes an instance. 404 → nil (idempotent). 409 retries
// for ~15 min UNLESS the body indicates a dependent replica blocks deletion.
func (c *ClientImpl) DeletePostgres(ctx context.Context, postgresId string) error {
	return c.deletePostgresWithInterval(ctx, postgresId, postgresDeleteRetryInterval, postgresDeleteRetryAttempts)
}

// deletePostgresWithInterval is the test seam for DeletePostgres.
func (c *ClientImpl) deletePostgresWithInterval(ctx context.Context, postgresId string, interval time.Duration, maxRetries uint64) error {
	deleteOnce := func() error {
		req, err := http.NewRequest(http.MethodDelete, c.getPostgresPath(postgresId, ""), nil)
		if err != nil {
			return backoff.Permanent(err)
		}
		_, err = c.doRequest(ctx, req)
		if err == nil {
			return nil
		}
		if IsNotFound(err) {
			return nil
		}
		if IsConflict(err) {
			if errIndicatesDependentReplica(err) {
				return backoff.Permanent(err)
			}
			return err
		}
		return backoff.Permanent(err)
	}
	if interval <= 0 {
		interval = postgresDeleteRetryInterval
	}
	return backoff.Retry(deleteOnce, backoff.WithMaxRetries(backoff.NewConstantBackOff(interval), maxRetries))
}

// errIndicatesDependentReplica fails fast on 409s caused by a read replica
// blocking primary deletion. AND-conjunction (not OR) avoids false-positives
// on transient conflicts like "replication slot exists" that retry resolves.
//
// FIXME: the keyword list is speculative — no real dev-cluster response has
// been captured yet. Once an integration test provisions a primary + replica
// and triggers a real 409, anchor this to a structured errorCode/reason field
// or the exact wording with a regression test. Until then, a real dependent-
// replica delete burns the full 15-min retry budget instead of failing fast.
func errIndicatesDependentReplica(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "409") {
		return false
	}
	return strings.Contains(msg, "depend") && strings.Contains(msg, "replica")
}

// ---------------------------------------------------------------------------
// WAIT helpers
// ---------------------------------------------------------------------------

// WaitForPostgresState polls GetPostgres every 5s until stateChecker returns
// true. 5xx bails permanently; other GetPostgres errors propagate verbatim;
// budget exhaustion surfaces the last seen state.
//
// maxWaitSeconds is a retry-count × poll-interval budget, NOT a wall-clock
// deadline — slow API responses push real elapsed time beyond the nominal
// limit. Size the resource `timeouts {}` block accordingly.
func (c *ClientImpl) WaitForPostgresState(ctx context.Context, postgresId string, stateChecker func(string) bool, maxWaitSeconds int) error {
	return c.waitForPostgresStateWithInterval(ctx, postgresId, stateChecker, postgresStatePollInterval, uint64(maxWaitSeconds/int(postgresStatePollInterval/time.Second))) //nolint:gosec
}

// waitForPostgresStateWithInterval is the test seam for WaitForPostgresState.
//
// check return values:
//
//	nil                          -> stateChecker matched; success
//	errPostgresStateNotYetTarget -> polled OK, target not yet hit; retry
//	backoff.Permanent(realErr)   -> 5xx; bail immediately
//	any other err                -> 4xx/transport/cancel; retry then bail
func (c *ClientImpl) waitForPostgresStateWithInterval(ctx context.Context, postgresId string, stateChecker func(string) bool, interval time.Duration, maxRetries uint64) error {
	var lastSeenState string
	check := func() error {
		pg, err := c.GetPostgres(ctx, postgresId)
		if is5xx(err) {
			return backoff.Permanent(err)
		} else if err != nil {
			return err
		}
		lastSeenState = pg.State
		if stateChecker(pg.State) {
			return nil
		}
		return errPostgresStateNotYetTarget
	}
	if maxRetries < 1 {
		maxRetries = 1
	}
	b := backoff.WithContext(backoff.NewConstantBackOff(interval), ctx)
	err := backoff.Retry(check, backoff.WithMaxRetries(b, maxRetries))
	if err == nil {
		return nil
	}
	if errors.Is(err, errPostgresStateNotYetTarget) {
		return fmt.Errorf("postgres %s did not reach the expected state in the allocated time (last seen state: %s)", postgresId, lastSeenState)
	}
	// Real GetPostgres error — propagate so callers can react via IsNotFound etc.
	return err
}

// WaitForPostgresMatch polls until predicate returns true for settleRetries
// consecutive polls. Caller precondition: mutating request MUST have
// returned 2xx. State-only checks race the Ubicloud queue; the predicate
// closes that race by gating on the requested field values.
func (c *ClientImpl) WaitForPostgresMatch(ctx context.Context, postgresId string, predicate func(*Postgres) bool, maxWaitSeconds int) error {
	maxRetries := uint64(maxWaitSeconds / int(postgresStatePollInterval/time.Second)) //nolint:gosec
	settleRetries := uint64(postgresStateSettleDuration / postgresStatePollInterval)  //nolint:gosec
	return c.waitForPostgresMatchWithInterval(ctx, postgresId, predicate, postgresStatePollInterval, maxRetries, settleRetries)
}

func (c *ClientImpl) waitForPostgresMatchWithInterval(ctx context.Context, postgresId string, predicate func(*Postgres) bool, interval time.Duration, maxRetries, settleRetries uint64) error {
	if settleRetries < 1 {
		settleRetries = 1
	}
	if maxRetries < settleRetries {
		maxRetries = settleRetries
	}
	var lastSeenState string
	var stableMatches uint64
	check := func() error {
		pg, err := c.GetPostgres(ctx, postgresId)
		if is5xx(err) {
			return backoff.Permanent(err)
		} else if err != nil {
			return err
		}
		lastSeenState = pg.State
		if predicate(pg) {
			stableMatches++
			if stableMatches >= settleRetries {
				return nil
			}
			return errPostgresStateUnchanged
		}
		stableMatches = 0
		return errPostgresStateNotYetTarget
	}
	b := backoff.WithContext(backoff.NewConstantBackOff(interval), ctx)
	err := backoff.Retry(check, backoff.WithMaxRetries(b, maxRetries))
	if err == nil {
		return nil
	}
	if errors.Is(err, errPostgresStateUnchanged) || errors.Is(err, errPostgresStateNotYetTarget) {
		return fmt.Errorf("postgres %s did not match the expected predicate within budget (last seen state: %s)", postgresId, lastSeenState)
	}
	return err
}

// ---------------------------------------------------------------------------
// PASSWORD
// ---------------------------------------------------------------------------

// SetPostgresPassword sets the superuser password. body.Password
// nil → server generates and returns one. body.Password set → server adopts
// it and returns empty.
//
// Idempotency caveat: re-PATCHing an empty body generates a NEW random
// password each time. Callers needing retry safety must persist the
// first-returned password before retrying.
func (c *ClientImpl) SetPostgresPassword(ctx context.Context, postgresId string, body PostgresPassword) (*PostgresPassword, error) {
	rb, err := json.Marshal(body) //nolint:gosec // Password is an intended request field, not a leak
	if err != nil {
		return nil, fmt.Errorf("failed to encode PostgresPassword: %w", err)
	}
	req, err := http.NewRequest(http.MethodPatch, c.getPostgresPath(postgresId, "/password"), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}
	respBody, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	resp := ResponseWithResult[PostgresPassword]{}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal PostgresPassword: %w", err)
	}
	return &resp.Result, nil
}

// ---------------------------------------------------------------------------
// CONFIG (Get / Replace)
// ---------------------------------------------------------------------------

// GetPostgresConfig returns user-set pgConfig / pgBouncerConfig parameters.
// Server defaults are not included in the response.
func (c *ClientImpl) GetPostgresConfig(ctx context.Context, postgresId string) (*PostgresConfig, error) {
	req, err := http.NewRequest(http.MethodGet, c.getPostgresPath(postgresId, "/config"), nil)
	if err != nil {
		return nil, err
	}
	respBody, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	resp := ResponseWithResult[PostgresConfig]{}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal PostgresConfig: %w", err)
	}
	return &resp.Result, nil
}

// ReplacePostgresConfig POSTs the full pgConfig + pgBouncerConfig. Keys
// absent from body are removed server-side; empty maps clear all parameters.
// Both fields are always sent as objects (defaulted to {} when nil) per the
// server's runtime validator at ManagedPostgresV1Handler.ts:643-646.
func (c *ClientImpl) ReplacePostgresConfig(ctx context.Context, postgresId string, body PostgresConfig) (*PostgresConfigUpdateResponse, error) {
	if body.PgConfig == nil {
		body.PgConfig = PgConfigMap{}
	}
	if body.PgBouncerConfig == nil {
		body.PgBouncerConfig = PgConfigMap{}
	}
	rb, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to encode PostgresConfig: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.getPostgresPath(postgresId, "/config"), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}
	respBody, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	resp := ResponseWithResult[PostgresConfigUpdateResponse]{}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal PostgresConfigUpdateResponse: %w", err)
	}
	return &resp.Result, nil
}

// ---------------------------------------------------------------------------
// RESTORE / READ REPLICA
// ---------------------------------------------------------------------------

// RestorePostgres creates a new instance from sourceId's backup at
// restoreTarget (RFC3339 timestamp).
func (c *ClientImpl) RestorePostgres(ctx context.Context, sourceId string, body PostgresRestoreRequest) (*Postgres, error) {
	rb, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to encode PostgresRestoreRequest: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.getPostgresPath(sourceId, "/restoredService"), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}
	respBody, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	resp := ResponseWithResult[Postgres]{}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Postgres: %w", err)
	}
	return &resp.Result, nil
}

// CreatePostgresReadReplica creates a read replica of the source primary.
func (c *ClientImpl) CreatePostgresReadReplica(ctx context.Context, sourceId string, body PostgresReadReplicaRequest) (*Postgres, error) {
	rb, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to encode PostgresReadReplicaRequest: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.getPostgresPath(sourceId, "/readReplica"), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}
	respBody, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	resp := ResponseWithResult[Postgres]{}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Postgres: %w", err)
	}
	return &resp.Result, nil
}

// ---------------------------------------------------------------------------
// CA CERTIFICATES (raw PEM response)
// ---------------------------------------------------------------------------

// GetPostgresCaCertificates returns the PEM-encoded CA chain. Bypasses
// doRequest because the server emits raw PEM (not JSON) and we don't want
// it flowing through the JSON pretty-printer.
//
// Trade-offs vs other client methods: no User-Agent header, no 429/5xx
// retry, no tflog logging, manual basic-auth setup.
//
// FIXME: when the matching data source lands, add a doRawRequest sibling in
// common.go that mirrors doRequest's retry/logging/auth machinery but returns
// bytes directly, and route this method through it. Public signature is stable.
func (c *ClientImpl) GetPostgresCaCertificates(ctx context.Context, postgresId string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.getPostgresPath(postgresId, "/caCertificates"), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.TokenKey, c.TokenSecret)
	res, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status: %d, body: %s", res.StatusCode, body)
	}
	return body, nil
}
