// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package s3_test

import (
	"context"
	"os"
	"testing"

	"github.com/leseb/openresponses-gw/pkg/filestore"
	"github.com/leseb/openresponses-gw/pkg/filestore/filestoretest"
	fss3 "github.com/leseb/openresponses-gw/pkg/filestore/s3"
)

func TestS3Conformance(t *testing.T) {
	bucket := os.Getenv("FILE_STORE_S3_BUCKET")
	endpoint := os.Getenv("FILE_STORE_S3_ENDPOINT")
	if bucket == "" || endpoint == "" {
		t.Skip("Skipping S3 conformance tests: FILE_STORE_S3_BUCKET and FILE_STORE_S3_ENDPOINT must be set (e.g. with MinIO)")
	}

	region := os.Getenv("FILE_STORE_S3_REGION")
	if region == "" {
		region = "us-east-1"
	}

	filestoretest.RunConformanceTests(t, func(t *testing.T) filestore.FileStore {
		store, err := fss3.New(context.Background(), fss3.Options{
			Bucket:   bucket,
			Region:   region,
			Prefix:   "test-" + t.Name() + "/",
			Endpoint: endpoint,
		})
		if err != nil {
			t.Fatalf("s3.New: %v", err)
		}
		return store
	})
}
