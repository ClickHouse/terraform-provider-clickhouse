package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const backupBucketTestPath = "/organizations/org-1/services/svc-1/backupBucket"

func TestGetBackupBucket(t *testing.T) {
	want := BackupBucket{Id: "b-1", BucketProvider: "AWS", BucketPath: "https://b.s3.amazonaws.com/p", IamRoleArn: "arn:aws:iam::1:role/r", IamRoleSessionName: "s"}

	client, _ := newUpgradeWindowTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != backupBucketTestPath {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[BackupBucket]{Result: want})
	})

	got, err := client.GetBackupBucket(context.Background(), "svc-1")
	if err != nil {
		t.Fatalf("GetBackupBucket: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestGetBackupBucket_NotFound(t *testing.T) {
	client, _ := newUpgradeWindowTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	_, err := client.GetBackupBucket(context.Background(), "svc-1")
	if !IsNotFound(err) {
		t.Fatalf("err = %v; want not found", err)
	}
}

func TestCreateAndUpdateBackupBucket_SendOnlyProviderFields(t *testing.T) {
	for _, tc := range []struct {
		method string
		call   func(*ClientImpl, BackupBucketRequest) (*BackupBucket, error)
	}{
		{http.MethodPost, func(c *ClientImpl, r BackupBucketRequest) (*BackupBucket, error) {
			return c.CreateBackupBucket(context.Background(), "svc-1", r)
		}},
		{http.MethodPatch, func(c *ClientImpl, r BackupBucketRequest) (*BackupBucket, error) {
			return c.UpdateBackupBucket(context.Background(), "svc-1", r)
		}},
	} {
		t.Run(tc.method, func(t *testing.T) {
			client, _ := newUpgradeWindowTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tc.method || r.URL.Path != backupBucketTestPath {
					t.Errorf("got %s %s", r.Method, r.URL.Path)
				}
				b, _ := io.ReadAll(r.Body)
				var got map[string]any
				if err := json.Unmarshal(b, &got); err != nil {
					t.Fatalf("body: %v", err)
				}
				want := map[string]any{"bucketProvider": "AZURE", "containerName": "c", "connectionString": "secret"}
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("body mismatch (-want +got):\n%s", diff)
				}
				_ = json.NewEncoder(w).Encode(ResponseWithResult[BackupBucket]{Result: BackupBucket{Id: "b-1", BucketProvider: "AZURE", ContainerName: "c"}})
			})

			c, n := "c", "secret"
			got, err := tc.call(client, BackupBucketRequest{BucketProvider: "AZURE", ContainerName: &c, ConnectionString: &n})
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got.Id != "b-1" || got.ContainerName != "c" {
				t.Errorf("got %+v", got)
			}
		})
	}
}

func TestDeleteBackupBucket(t *testing.T) {
	called := false
	client, _ := newUpgradeWindowTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodDelete || r.URL.Path != backupBucketTestPath {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{}`))
	})

	if err := client.DeleteBackupBucket(context.Background(), "svc-1"); err != nil {
		t.Fatalf("DeleteBackupBucket: %v", err)
	}
	if !called {
		t.Error("handler not called")
	}
}
