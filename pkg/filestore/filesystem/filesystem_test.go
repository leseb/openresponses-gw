// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package filesystem_test

import (
	"testing"

	"github.com/leseb/openresponses-gw/pkg/filestore"
	"github.com/leseb/openresponses-gw/pkg/filestore/filestoretest"
	"github.com/leseb/openresponses-gw/pkg/filestore/filesystem"
)

func TestFilesystemConformance(t *testing.T) {
	filestoretest.RunConformanceTests(t, func(t *testing.T) filestore.FileStore {
		store, err := filesystem.New(t.TempDir())
		if err != nil {
			t.Fatalf("filesystem.New: %v", err)
		}
		return store
	})
}
