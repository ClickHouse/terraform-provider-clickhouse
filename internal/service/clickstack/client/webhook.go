package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const webhooksPath = "/api/v2/webhooks"

// webhookListPageSize is the page size used when listing webhooks. The v2
// webhooks endpoint has no GET-by-id, so GetWebhook lists and filters; listing
// pages through all results so a webhook beyond the first page is still found.
const webhookListPageSize = 200

// webhookListMaxPages bounds ListWebhooks against a pathological server that
// never converges. At webhookListPageSize this covers ~2M webhooks.
const webhookListMaxPages = 10000

// Webhook is a ClickStack notification webhook as exchanged with the v2 API.
//
// Headers and QueryParams are secret-bearing and write-only: the API accepts
// them on create/update but never returns them, so they are always nil on a
// read. Body is returned only for the generic service. The same struct is used
// for both requests and responses; write-only fields are simply absent when
// decoding a response.
type Webhook struct {
	ID          string            `json:"id,omitempty"`
	Service     string            `json:"service"`
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	Description *string           `json:"description,omitempty"`
	QueryParams map[string]string `json:"queryParams,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        *string           `json:"body,omitempty"`
}

// webhookEnvelope wraps single-webhook API responses.
type webhookEnvelope struct {
	Data Webhook `json:"data"`
}

// listMeta is the pagination metadata returned by list endpoints.
type listMeta struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// webhookListEnvelope wraps webhook-list API responses.
type webhookListEnvelope struct {
	Data []Webhook `json:"data"`
	Meta *listMeta `json:"meta,omitempty"`
}

// CreateWebhook creates a webhook and returns it as stored by the API. The
// write-only Headers/QueryParams are sent but will not be present on the
// returned webhook.
func (c *Client) CreateWebhook(ctx context.Context, input Webhook) (*Webhook, error) {
	// The body carries secret-bearing headers/queryParams: that is the API
	// contract for provisioning a webhook. It is sent over the authenticated
	// connection and never logged.
	body, err := json.Marshal(input) //nolint:gosec // G117: secrets in body are the API contract
	if err != nil {
		return nil, fmt.Errorf("encode webhook: %w", err)
	}

	raw, err := c.do(ctx, http.MethodPost, webhooksPath, body)
	if err != nil {
		return nil, err
	}

	var resp webhookEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode webhook: %w", err)
	}
	return &resp.Data, nil
}

// eachWebhook pages through the webhook list, invoking fn for each distinct
// webhook. fn returns true to stop early (used by GetWebhook to stop at the first
// match instead of walking the whole list every refresh).
//
// Termination: a page that adds nothing new (empty or all-seen — a server that
// ignores offset) stops the walk, as does reaching meta.total. A short page is
// NOT treated as the last page when there is no meta: a server that caps its page
// size below the requested limit returns a short first page even when more
// webhooks exist, and stopping there would under-read. A hard page cap bounds the
// pathological case of endless distinct IDs with no meta.
func (c *Client) eachWebhook(ctx context.Context, fn func(Webhook) bool) error {
	seen := map[string]bool{}
	offset, total := 0, 0
	for page := 0; ; page++ {
		if page >= webhookListMaxPages {
			return fmt.Errorf("list webhooks: pagination did not converge after %d pages", webhookListMaxPages)
		}
		q := url.Values{}
		q.Set("limit", strconv.Itoa(webhookListPageSize))
		q.Set("offset", strconv.Itoa(offset))

		raw, err := c.do(ctx, http.MethodGet, webhooksPath+"?"+q.Encode(), nil)
		if err != nil {
			return err
		}
		var resp webhookListEnvelope
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("decode webhooks: %w", err)
		}

		fresh := 0
		for i := range resp.Data {
			// Only a not-yet-seen, identified webhook counts as progress. Empty-id
			// rows (malformed; the real API always assigns ids) are still passed to
			// fn but do not count toward `fresh`, so a page that adds no new
			// identified rows converges (fresh == 0) instead of looping to the cap
			// on an offset-ignoring server.
			if id := resp.Data[i].ID; id != "" {
				if seen[id] {
					continue
				}
				seen[id] = true
				fresh++
			}
			total++
			if fn(resp.Data[i]) {
				return nil
			}
		}
		if fresh == 0 {
			break
		}
		// Advance by the number of rows the server returned, not by the count of
		// distinct-new IDs: on a well-behaved server these are equal, but a page
		// carrying a duplicate or empty-id row would otherwise under-advance the
		// offset and re-request the overlap. The seen-map dedup and the fresh==0
		// convergence check above still bound an offset-ignoring server.
		offset += len(resp.Data)
		if resp.Meta != nil && total >= resp.Meta.Total {
			break
		}
	}
	return nil
}

// ListWebhooks fetches all webhooks for the authenticated team, paging through
// every result. The v2 API exposes no GET-by-id, so callers that want one webhook
// use GetWebhook.
func (c *Client) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	var all []Webhook
	err := c.eachWebhook(ctx, func(wh Webhook) bool {
		all = append(all, wh)
		return false
	})
	if err != nil {
		return nil, err
	}
	return all, nil
}

// GetWebhook fetches a single webhook by ID. The v2 API has no GET-by-id
// endpoint, so this pages the webhook list and returns as soon as the ID matches.
// It returns an error wrapping ErrNotFound when no webhook has that ID.
func (c *Client) GetWebhook(ctx context.Context, id string) (*Webhook, error) {
	var found *Webhook
	err := c.eachWebhook(ctx, func(wh Webhook) bool {
		if wh.ID == id {
			w := wh
			found = &w
			return true
		}
		return false
	})
	if err != nil {
		return nil, err
	}
	if found == nil {
		return nil, fmt.Errorf("get webhook %s: %w", id, ErrNotFound)
	}
	return found, nil
}

// UpdateWebhook updates a webhook by ID and returns the updated webhook. The API
// PUT is a partial $set/$unset of the provided fields, not a whole-document
// replace: name/url/service are always written; description/body are cleared when
// omitted; and the write-only headers/queryParams are KEPT when omitted, EXCEPT
// when url or service changes (the server clears omitted secrets on a destination
// change, so they must be re-supplied). It returns an error wrapping ErrNotFound
// when the webhook does not exist.
func (c *Client) UpdateWebhook(ctx context.Context, id string, input Webhook) (*Webhook, error) {
	// See CreateWebhook: secrets in the body are the API contract.
	body, err := json.Marshal(input) //nolint:gosec // G117: secrets in body are the API contract
	if err != nil {
		return nil, fmt.Errorf("encode webhook: %w", err)
	}

	raw, err := c.do(ctx, http.MethodPut, webhooksPath+"/"+url.PathEscape(id), body)
	if err != nil {
		return nil, err
	}

	var resp webhookEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode webhook: %w", err)
	}
	return &resp.Data, nil
}

// DeleteWebhook deletes a webhook by ID. It returns an error wrapping
// ErrNotFound when the webhook does not exist. The API returns 409 while an
// alert still references the webhook; that surfaces as a non-nil error carrying
// the API message.
func (c *Client) DeleteWebhook(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, webhooksPath+"/"+url.PathEscape(id), nil)
	return err
}
