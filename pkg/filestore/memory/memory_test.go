// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package memory_test

import (
	"testing"

	"github.com/leseb/openresponses-gw/pkg/filestore"
	"github.com/leseb/openresponses-gw/pkg/filestore/filestoretest"
	"github.com/leseb/openresponses-gw/pkg/filestore/memory"
)

func TestMemoryConformance(t *testing.T) {
	filestoretest.RunConformanceTests(t, func(t *testing.T) filestore.FileStore {
		return memory.New()
	})
}
