// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

// Package provider implements a generic factory registry for pluggable backends.
//
// Each subsystem (filestore, vectorstore, session store, websearch) creates
// a typed Registry and implementations self-register via init(). This follows
// the database/sql driver pattern: blank-import an implementation package to
// activate it, then call Registry.New(name, params) to instantiate.
package provider

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Factory is a constructor function that creates a backend instance from
// a string parameter map. Implementations extract the keys they need and
// ignore the rest.
type Factory[T any] func(ctx context.Context, params map[string]string) (T, error)

// Registry is a thread-safe registry of named factory functions for a
// given backend interface T.
type Registry[T any] struct {
	subsystem string
	mu        sync.RWMutex
	factories map[string]Factory[T]
}

// NewRegistry creates a new Registry. The subsystem name is used in error
// messages (e.g. "file_store", "session_store").
func NewRegistry[T any](subsystem string) *Registry[T] {
	return &Registry[T]{
		subsystem: subsystem,
		factories: make(map[string]Factory[T]),
	}
}

// Register adds a named factory. Panics if the name is already registered
// (catches duplicate init() registrations at startup).
func (r *Registry[T]) Register(name string, f Factory[T]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[name]; exists {
		panic(fmt.Sprintf("provider: %s backend %q already registered", r.subsystem, name))
	}
	r.factories[name] = f
}

// New creates a backend instance by name. Returns an error if the name
// is not registered.
func (r *Registry[T]) New(ctx context.Context, name string, params map[string]string) (T, error) {
	r.mu.RLock()
	f, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		var zero T
		return zero, fmt.Errorf("unknown %s provider: %q (available: %v)", r.subsystem, name, r.Available())
	}
	return f(ctx, params)
}

// Available returns the sorted list of registered backend names.
func (r *Registry[T]) Available() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
