// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"testing"
)

func TestNewProcessor(t *testing.T) {
	// Test with nil engine and logger (should not panic)
	proc := NewProcessor(nil, nil)
	if proc == nil {
		t.Fatal("expected non-nil processor")
	}
	if proc.logger == nil {
		t.Error("expected non-nil logger")
	}
	if proc.injector == nil {
		t.Error("expected non-nil injector")
	}
}

func TestGenerateRequestID(t *testing.T) {
	// Test format
	id := generateRequestID()
	if len(id) < 5 {
		t.Errorf("expected ID with prefix, got %q", id)
	}
	if id[:4] != "req_" {
		t.Errorf("expected prefix req_, got %q", id[:4])
	}

	// Test uniqueness
	id2 := generateRequestID()
	if id == id2 {
		t.Error("expected unique IDs")
	}
}
