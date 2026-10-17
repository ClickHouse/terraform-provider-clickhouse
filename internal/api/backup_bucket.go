package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

// Backup bucket providers accepted by the `bucketProvider` discriminator.
const (
	BackupBucketProviderAWS   = "AWS"
	BackupBucketProviderGCP   = "GCP"
	BackupBucketProviderAzure = "AZURE"
)

// BackupBucket mirrors the OpenAPI BackupBucket `oneOf` (AWS, GCP, AZURE).
// Fields not belonging to the active provider are empty. Secrets
// (`secretAccessKey`, `connectionString`) are never returned by the API.
type BackupBucket struct {
	Id                 string `json:"id"`
	BucketProvider     string `json:"bucketProvider"`
	BucketPath         string `json:"bucketPath,omitempty"`
	IamRoleArn         string `json:"iamRoleArn,omitempty"`
	IamRoleSessionName string `json:"iamRoleSessionName,omitempty"`
	AccessKeyId        string `json:"accessKeyId,omitempty"`
	ContainerName      string `json:"containerName,omitempty"`
}

// BackupBucketRequest is the POST/PATCH body. Only the fields of the chosen
// provider variant must be set; the rest are omitted from the payload.
type BackupBucketRequest struct {
	BucketProvider     string  `json:"bucketProvider"`
	BucketPath         *string `json:"bucketPath,omitempty"`
	IamRoleArn         *string `json:"iamRoleArn,omitempty"`
	IamRoleSessionName *string `json:"iamRoleSessionName,omitempty"`
	AccessKeyId        *string `json:"accessKeyId,omitempty"`
	SecretAccessKey    *string `json:"secretAccessKey,omitempty"`
	ContainerName      *string `json:"containerName,omitempty"`
	ConnectionString   *string `json:"connectionString,omitempty"`
}

func (c *ClientImpl) GetBackupBucket(ctx context.Context, serviceId string) (*BackupBucket, error) {
	return c.doBackupBucket(ctx, http.MethodGet, serviceId, nil)
}

func (c *ClientImpl) CreateBackupBucket(ctx context.Context, serviceId string, r BackupBucketRequest) (*BackupBucket, error) {
	return c.doBackupBucket(ctx, http.MethodPost, serviceId, &r)
}

func (c *ClientImpl) UpdateBackupBucket(ctx context.Context, serviceId string, r BackupBucketRequest) (*BackupBucket, error) {
	return c.doBackupBucket(ctx, http.MethodPatch, serviceId, &r)
}

func (c *ClientImpl) DeleteBackupBucket(ctx context.Context, serviceId string) error {
	req, err := http.NewRequest(http.MethodDelete, c.getServicePath(serviceId, "/backupBucket"), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(ctx, req)
	return err
}

func (c *ClientImpl) doBackupBucket(ctx context.Context, method, serviceId string, r *BackupBucketRequest) (*BackupBucket, error) {
	var reqBody *bytes.Reader
	if r != nil {
		rb, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(rb)
	}

	var (
		req *http.Request
		err error
	)
	if reqBody != nil {
		req, err = http.NewRequest(method, c.getServicePath(serviceId, "/backupBucket"), reqBody)
	} else {
		req, err = http.NewRequest(method, c.getServicePath(serviceId, "/backupBucket"), nil)
	}
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[BackupBucket]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Result, nil
}
