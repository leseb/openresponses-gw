// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Connector represents a stored MCP connector
type Connector struct {
	ConnectorID   string
	ConnectorType string
	URL           string
	ServerLabel   string
	CreatedAt     time.Time
	Metadata      map[string]string
}

// ConnectorsStore is an in-memory connectors store
type ConnectorsStore struct {
	mu         sync.RWMutex
	connectors map[string]*Connector // keyed by ConnectorID
}

// NewConnectorsStore creates a new connectors store
func NewConnectorsStore() *ConnectorsStore {
	return &ConnectorsStore{
		connectors: make(map[string]*Connector),
	}
}

// CreateConnector creates or overwrites a connector
func (s *ConnectorsStore) CreateConnector(ctx context.Context, connector *Connector) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.connectors[connector.ConnectorID] = connector
	return nil
}

// GetConnector retrieves a connector by ID
func (s *ConnectorsStore) GetConnector(ctx context.Context, connectorID string) (*Connector, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	connector, exists := s.connectors[connectorID]
	if !exists {
		return nil, fmt.Errorf("connector %s not found", connectorID)
	}

	return connector, nil
}

// DeleteConnector deletes a connector
func (s *ConnectorsStore) DeleteConnector(ctx context.Context, connectorID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.connectors[connectorID]; !exists {
		return fmt.Errorf("connector %s not found", connectorID)
	}

	delete(s.connectors, connectorID)
	return nil
}

// ListConnectorsPaginated lists connectors with pagination
func (s *ConnectorsStore) ListConnectorsPaginated(ctx context.Context, after, before string, limit int, order string) ([]*Connector, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Default limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Collect all connectors
	var allConnectors []*Connector
	for _, connector := range s.connectors {
		allConnectors = append(allConnectors, connector)
	}

	// Apply cursor-based pagination
	var filtered []*Connector
	foundAfter := after == ""

	for _, connector := range allConnectors {
		// Handle after cursor
		if after != "" && !foundAfter {
			if connector.ConnectorID == after {
				foundAfter = true
			}
			continue
		}

		// Handle before cursor
		if before != "" && connector.ConnectorID == before {
			break
		}

		filtered = append(filtered, connector)

		// Limit results
		if len(filtered) >= limit {
			break
		}
	}

	// Check if there are more results
	hasMore := len(allConnectors) > len(filtered) && len(filtered) == limit

	return filtered, hasMore, nil
}
