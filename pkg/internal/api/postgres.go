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

// errPostgresStateUnchanged signals that a poll observed the terminal state
// during a leave-and-return wait. It is treated as a legitimate "no-op
// success" by the caller; any OTHER error from GetPostgres (404, 401/403,
// 5xx, context cancellation) propagates instead.
var errPostgresStateUnchanged = errors.New("postgres state unchanged from terminal")

// Default retry budgets for Postgres operations. Exposed as package-level
// vars so callers (e.g., resource Delete with a custom `timeouts {}` block)
// can override them per-call via the *WithBudget / *WithInterval variants.
var (
	postgresDeleteRetryInterval   = 10 * time.Second
	postgresDeleteRetryBudgetSecs = 15 * 60 // 15 minutes
	postgresStatePollInterval     = 5 * time.Second
)

// nonNegU64 clamps an int to a non-negative uint64. Used when converting
// caller-supplied time budgets to retry counts.
func nonNegU64(n int) uint64 {
	if n <= 0 {
		return 0
	}
	return uint64(n)
}

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
		return nil, err
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
		return nil, err
	}
	return resp.Result, nil
}

// ---------------------------------------------------------------------------
// CREATE / UPDATE / DELETE
// ---------------------------------------------------------------------------

// CreatePostgres provisions a new Postgres instance.
// Returns the server-side instance plus a server-generated password as a
// separate return value. The password is non-nil only when the server
// generated one (i.e., the request body had no password set). The GET
// endpoint never echoes the password, so callers must persist it from
// this response before any subsequent operation that could fail.
func (c *ClientImpl) CreatePostgres(ctx context.Context, body PostgresCreate) (*Postgres, *string, error) {
	rb, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.getPostgresPath("", ""), bytes.NewReader(rb))
	if err != nil {
		return nil, nil, err
	}
	respBody, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	resp := ResponseWithResult[Postgres]{}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, nil, err
	}
	var generatedPassword *string
	if resp.Result.Password != nil {
		// Copy out so the caller has a stable pointer and the model can be
		// returned without leaking the plaintext into log paths that don't
		// already redact it.
		copyVal := *resp.Result.Password
		generatedPassword = &copyVal
	}
	return &resp.Result, generatedPassword, nil
}

// UpdatePostgres applies a PATCH to a Postgres instance. The server accepts
// only `size`, `haType`, `tags`; the Go-side PostgresUpdate type enforces
// this.
func (c *ClientImpl) UpdatePostgres(ctx context.Context, postgresId string, body PostgresUpdate) (*Postgres, error) {
	rb, err := json.Marshal(body)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	return &resp.Result, nil
}

// DeletePostgres deletes a Postgres instance. 404 → nil (idempotent).
// 409 is retried with constant backoff for up to ~15 min UNLESS the response
// body indicates a dependent replica exists, in which case it fails fast.
func (c *ClientImpl) DeletePostgres(ctx context.Context, postgresId string) error {
	return c.deletePostgresWithBudget(ctx, postgresId, postgresDeleteRetryInterval,
		nonNegU64(postgresDeleteRetryBudgetSecs)/nonNegU64(int(postgresDeleteRetryInterval/time.Second)))
}

// deletePostgresWithBudget is the parameterized core of DeletePostgres,
// exposed for unit tests so 409 retry can be exercised in milliseconds rather
// than minutes.
func (c *ClientImpl) deletePostgresWithBudget(ctx context.Context, postgresId string, interval time.Duration, maxRetries uint64) error {
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
			// Fail fast when the conflict is due to a dependent replica.
			// Retrying never resolves it and burns the budget.
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

// errIndicatesDependentReplica inspects an error from doRequest (formatted as
// "status: N, body: {...}") for the conjunction of "depend" and "replica" in
// the message, which the server uses to signal that a read replica blocks
// deletion of its primary.
//
// The conjunction is deliberate: matching only "replica" (e.g., for a 409
// containing "replication slot exists") would cause false-positive fail-fast
// behavior on transient conflicts that retry could resolve.
//
// FIXME(phase-6): the keyword list is speculative. The Phase 6 integration
// test `tests/postgres/drift/` should provision a primary + replica, attempt
// to delete the primary, and capture the verbatim 409 response. Update this
// heuristic to anchor to a stable structured field (errorCode/reason if the
// server provides one) or to the captured wording with a regression test.
// Worst case until then: a dependent-replica delete burns the full 15-min
// retry budget instead of failing fast.
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

// WaitForPostgresState polls GetPostgres until stateChecker returns true.
// Polls every 5 seconds; success on match; permanent failure on 5xx; surfaces
// the last seen state in the timeout error.
//
// maxWaitSeconds is a retry-count × poll-interval budget, NOT a wall-clock
// deadline — slow API responses push real elapsed time beyond the nominal
// limit. Users tuning the resource's `timeouts {}` block should size this
// as a poll budget.
func (c *ClientImpl) WaitForPostgresState(ctx context.Context, postgresId string, stateChecker func(string) bool, maxWaitSeconds int) error {
	return c.waitForPostgresStateWithInterval(ctx, postgresId, stateChecker, postgresStatePollInterval, nonNegU64(maxWaitSeconds/int(postgresStatePollInterval/time.Second)))
}

// waitForPostgresStateWithInterval is the parameterized core, exposed for
// unit tests.
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
		return fmt.Errorf("postgres %s is in state %s", postgresId, pg.State)
	}
	if maxRetries < 1 {
		maxRetries = 1
	}
	err := backoff.Retry(check, backoff.WithMaxRetries(backoff.NewConstantBackOff(interval), maxRetries))
	if err == nil {
		return nil
	}
	return fmt.Errorf("postgres %s did not reach the expected state in the allocated time (last seen state: %s)", postgresId, lastSeenState)
}

// WaitForPostgresLeaveAndReturn waits for state to leave terminalState and
// then come back to it. Used after PATCH operations (size change, ha_type
// change, config replace) to avoid the race where the API responds before
// the transition begins server-side.
//
// **Caller precondition:** the mutating request that should trigger the
// transition MUST have already returned a 2xx from the server. This helper
// treats "state never left terminal" as a no-op success (e.g., a config
// change that hot-reloaded without a restart). If you invoke it after a
// silently-failed mutation, it will report success even though nothing
// happened — so verify the PATCH/POST return value first.
//
// FIXME(phase-2): the "never left = success" fallback can also miss a real
// race — if the server hasn't started transitioning by the time leave-detection
// exhausts its budget, we return success and the caller proceeds before the
// actual transition. Phase 2's resource Update will exercise this for real
// against the dev cluster. If the race is observable, add a minimum observation
// window (always poll ≥ N times before concluding "no transition") with N
// anchored to measured transition latency. The `*WithInterval` test seam
// already supports parameterizing the window.
func (c *ClientImpl) WaitForPostgresLeaveAndReturn(ctx context.Context, postgresId string, terminalState string, maxWaitSeconds int) error {
	return c.waitForPostgresLeaveAndReturnWithInterval(ctx, postgresId, terminalState, postgresStatePollInterval, nonNegU64(maxWaitSeconds/int(postgresStatePollInterval/time.Second)))
}

func (c *ClientImpl) waitForPostgresLeaveAndReturnWithInterval(ctx context.Context, postgresId string, terminalState string, interval time.Duration, maxRetries uint64) error {
	// Phase 1: wait until state differs from terminalState.
	//
	// leftCheck distinguishes three outcomes via its return error:
	//   nil                            -> state has left terminalState; advance to phase 2
	//   errPostgresStateUnchanged      -> polled successfully, state still terminal; retry
	//   backoff.Permanent(realErr)     -> 5xx; bail
	//   any other err                  -> 4xx/transport/cancel; retry up to budget, then bail
	left := false
	leftCheck := func() error {
		pg, err := c.GetPostgres(ctx, postgresId)
		if is5xx(err) {
			return backoff.Permanent(err)
		} else if err != nil {
			return err
		}
		if pg.State != terminalState {
			left = true
			return nil
		}
		return errPostgresStateUnchanged
	}
	if maxRetries < 2 {
		maxRetries = 2
	}
	halfBudget := maxRetries / 2
	if halfBudget < 1 {
		halfBudget = 1
	}
	err := backoff.Retry(leftCheck, backoff.WithMaxRetries(backoff.NewConstantBackOff(interval), halfBudget))
	if err != nil && !left {
		// Only the sentinel-caused exhaustion is a no-op success. Anything
		// else (404, 401/403, context cancellation, exhausted 5xx) is a real
		// failure and the caller must see it — otherwise the resource layer
		// would proceed as if the update succeeded when polling itself failed.
		if errors.Is(err, errPostgresStateUnchanged) {
			return nil
		}
		return err
	}
	// Phase 2: wait until state returns to terminalState.
	return c.waitForPostgresStateWithInterval(ctx, postgresId, func(s string) bool { return s == terminalState }, interval, maxRetries-halfBudget)
}

// ---------------------------------------------------------------------------
// PASSWORD
// ---------------------------------------------------------------------------

// SetPostgresPassword sets (or rotates) the Postgres superuser password.
// When body.Password is non-nil, the server adopts that value and returns an
// empty response. When body.Password is nil, the server generates a fresh
// random password and returns it in the response.
//
// Idempotency caveat: re-PATCHing the same user-supplied password is safe.
// Re-PATCHing an empty body always generates a NEW random password — callers
// requiring retry safety must persist the first-returned password before
// retrying.
func (c *ClientImpl) SetPostgresPassword(ctx context.Context, postgresId string, body PostgresPassword) (*PostgresPassword, error) {
	rb, err := json.Marshal(body)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	return &resp.Result, nil
}

// ---------------------------------------------------------------------------
// CONFIG (Get / Replace POST / Update PATCH)
// ---------------------------------------------------------------------------

// GetPostgresConfig fetches the current pgConfig and pgBouncerConfig.
// Returns only the parameters set by the user; server defaults are not
// included in the response.
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
		return nil, err
	}
	return &resp.Result, nil
}

// ReplacePostgresConfig replaces the entire pgConfig and pgBouncerConfig
// for an instance. Full replacement — keys absent from body are removed
// server-side. Empty maps are accepted (clears all parameters).
func (c *ClientImpl) ReplacePostgresConfig(ctx context.Context, postgresId string, body PostgresConfig) (*PostgresConfigUpdateResponse, error) {
	return c.configMutate(ctx, postgresId, body, http.MethodPost)
}

// UpdatePostgresConfig applies a PATCH-merge to the config maps. Keys
// absent from body stay as-is server-side. Provided for completeness;
// the Phase 3 resource uses ReplacePostgresConfig instead because the
// PATCH-merge semantics don't fit Terraform's declarative model.
func (c *ClientImpl) UpdatePostgresConfig(ctx context.Context, postgresId string, body PostgresConfig) (*PostgresConfigUpdateResponse, error) {
	return c.configMutate(ctx, postgresId, body, http.MethodPatch)
}

func (c *ClientImpl) configMutate(ctx context.Context, postgresId string, body PostgresConfig, method string) (*PostgresConfigUpdateResponse, error) {
	// Always send both pgConfig and pgBouncerConfig as objects (possibly
	// empty). The server's runtime validator requires both fields present;
	// see Phase 0 Curl 3 + ManagedPostgresV1Handler.ts:643-646.
	if body.PgConfig == nil {
		body.PgConfig = PgConfigMap{}
	}
	if body.PgBouncerConfig == nil {
		body.PgBouncerConfig = PgConfigMap{}
	}
	rb, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, c.getPostgresPath(postgresId, "/config"), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}
	respBody, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	resp := ResponseWithResult[PostgresConfigUpdateResponse]{}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, err
	}
	return &resp.Result, nil
}

// ---------------------------------------------------------------------------
// RESTORE / READ REPLICA / STATE COMMAND
// ---------------------------------------------------------------------------

// RestorePostgres creates a new Postgres instance restored from the source
// instance's backup at restoreTarget (RFC3339 timestamp).
func (c *ClientImpl) RestorePostgres(ctx context.Context, sourceId string, body PostgresRestoreRequest) (*Postgres, error) {
	rb, err := json.Marshal(body)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	return &resp.Result, nil
}

// CreatePostgresReadReplica creates a read replica of the source primary.
func (c *ClientImpl) CreatePostgresReadReplica(ctx context.Context, sourceId string, body PostgresReadReplicaRequest) (*Postgres, error) {
	rb, err := json.Marshal(body)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	return &resp.Result, nil
}

// PostgresStateCommandSend issues a state command (restart / promote /
// switchover). Exposed on the client for completeness; the Phase 2 resource
// does not call this — operational commands are intentionally excluded from
// the resource's declarative surface in v1.
func (c *ClientImpl) PostgresStateCommandSend(ctx context.Context, postgresId string, command string) (*Postgres, error) {
	rb, err := json.Marshal(PostgresStateCommandRequest{Command: command})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPatch, c.getPostgresPath(postgresId, "/state"), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}
	respBody, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	resp := ResponseWithResult[Postgres]{}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, err
	}
	return &resp.Result, nil
}

// ---------------------------------------------------------------------------
// CA CERTIFICATES (raw PEM response)
// ---------------------------------------------------------------------------

// GetPostgresCaCertificates fetches the CA certificate chain for the instance
// in PEM format. The endpoint's server-side handler bypasses the standard
// response envelope wrapping, so we issue the request directly through the
// HTTP client instead of doRequest (which assumes a JSON response and would
// otherwise emit a PEM blob through the JSON pretty-printer into tflog).
//
// **Operational trade-offs vs. every other client method:**
//   - No `User-Agent: terraform-provider-clickhouse/{version}` header — this
//     endpoint shows up as anonymous in server-side traffic attribution.
//   - No 429 / 5xx retry (transient hiccups return immediately).
//   - No tflog request/response logging.
//   - Bypasses the centralized basic-auth setup.
//
// FIXME(phase-5): the first Terraform consumer of this method is Phase 5's
// `data.clickhouse_postgres_service_ca_certificates` data source. When that
// lands, add a sibling `doRawRequest` helper in common.go that mirrors
// doRequest's retry / logging / auth / User-Agent machinery but returns bytes
// directly without JSON envelope decoding, and route this method through it.
// The signature `GetPostgresCaCertificates(ctx, id) ([]byte, error)` is stable;
// only the implementation changes.
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
