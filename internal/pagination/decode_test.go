package pagination

import (
	"reflect"
	"testing"
)

func TestDecodeOpenAPIPage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		want    Page[item]
		wantErr bool
	}{
		{
			name: "unpaginated legacy shape is a last page",
			body: `{"status":200,"requestId":"r1","result":[{"ID":"a"}]}`,
			want: Page[item]{Items: items("a")},
		},
		{
			name: "paginated page",
			body: `{"status":200,"requestId":"r1","result":[{"ID":"a"}],"limit":50,"totalCount":120,"nextCursor":"abc"}`,
			want: Page[item]{Items: items("a"), Limit: 50, TotalCount: intPtr(120), NextCursor: "abc"},
		},
		{
			name: "last page with null cursor and null total",
			body: `{"result":[{"ID":"a"}],"limit":50,"totalCount":null,"nextCursor":null}`,
			want: Page[item]{Items: items("a"), Limit: 50},
		},
		{
			name: "empty result",
			body: `{"result":[],"limit":50,"totalCount":0}`,
			want: Page[item]{Items: []item{}, Limit: 50, TotalCount: intPtr(0)},
		},
		{
			name:    "result is not a list",
			body:    `{"result":{"items":[]}}`,
			wantErr: true,
		},
		{
			name:    "malformed",
			body:    `not json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := DecodeOpenAPIPage[item]([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("page = %+v, want %+v", got, tt.want)
			}
		})
	}
}
