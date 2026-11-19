package api

import (
	"context"
	"net/url"
	"testing"
)

func TestGetApiKeyID_Paginated(t *testing.T) {
	t.Parallel()

	pages := map[string]string{
		"":   `{"result":[{"id":"k1","name":"first","keySuffix":"zzzz"}],"limit":1,"nextCursor":"c1"}`,
		"c1": `{"result":[{"id":"k2","name":"second","keySuffix":"yyyy"}],"limit":1,"nextCursor":"c2"}`,
		"c2": `{"result":[{"id":"k3","name":"third","keySuffix":"_key"}],"limit":1,"nextCursor":null}`,
	}

	tests := []struct {
		name      string
		keyName   *string
		wantID    string
		wantPages int
		wantErr   string
	}{
		{name: "match by name on the first page stops early", keyName: strPtr("first"), wantID: "k1", wantPages: 1},
		{name: "match by name on a later page", keyName: strPtr("second"), wantID: "k2", wantPages: 2},
		{name: "match by own token suffix on the last page", wantID: "k3", wantPages: 3},
		{name: "no match walks every page", keyName: strPtr("missing"), wantPages: 3, wantErr: `API key named "missing" was not found`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var queries []url.Values
			c, _ := newTestClient(t, cursorPages(t, pages, &queries))
			c.TokenKey = "my_key" // suffix-matches k3

			key, err := c.GetApiKeyID(context.Background(), tt.keyName)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("expected error %q, got %v", tt.wantErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("GetApiKeyID: %v", err)
				}
				if key.ID != tt.wantID {
					t.Errorf("key id = %q, want %q", key.ID, tt.wantID)
				}
			}
			if len(queries) != tt.wantPages {
				t.Errorf("fetched %d pages, want %d", len(queries), tt.wantPages)
			}
		})
	}
}
