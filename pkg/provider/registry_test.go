// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"testing"
)

type mockBackend struct{ name string }

func TestRegistry_RegisterAndNew(t *testing.T) {
	r := NewRegistry[*mockBackend]("test")
	r.Register("alpha", func(_ context.Context, params map[string]string) (*mockBackend, error) {
		return &mockBackend{name: params["name"]}, nil
	})

	b, err := r.New(context.Background(), "alpha", map[string]string{"name": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.name != "hello" {
		t.Errorf("expected name 'hello', got %q", b.name)
	}
}

func TestRegistry_UnknownProvider(t *testing.T) {
	r := NewRegistry[*mockBackend]("widget")
	r.Register("a", func(_ context.Context, _ map[string]string) (*mockBackend, error) {
		return &mockBackend{}, nil
	})

	_, err := r.New(context.Background(), "z", nil)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	want := `unknown widget provider: "z" (available: [a])`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestRegistry_Available(t *testing.T) {
	r := NewRegistry[*mockBackend]("test")
	r.Register("bravo", func(_ context.Context, _ map[string]string) (*mockBackend, error) {
		return &mockBackend{}, nil
	})
	r.Register("alpha", func(_ context.Context, _ map[string]string) (*mockBackend, error) {
		return &mockBackend{}, nil
	})

	avail := r.Available()
	if len(avail) != 2 || avail[0] != "alpha" || avail[1] != "bravo" {
		t.Errorf("Available() = %v, want [alpha bravo]", avail)
	}
}

func TestRegistry_DuplicatePanics(t *testing.T) {
	r := NewRegistry[*mockBackend]("test")
	r.Register("dup", func(_ context.Context, _ map[string]string) (*mockBackend, error) {
		return &mockBackend{}, nil
	})

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	r.Register("dup", func(_ context.Context, _ map[string]string) (*mockBackend, error) {
		return &mockBackend{}, nil
	})
}
