// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/leseb/openresponses-gw/docs"
	"gopkg.in/yaml.v3"
)

var (
	cachedJSON []byte
	jsonOnce   sync.Once
)

// handleOpenAPI serves the OpenAPI specification as JSON.
// The spec is generated from Go annotations via swag and embedded at build time.
func (h *Handler) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	jsonOnce.Do(func() {
		var spec interface{}
		if err := yaml.Unmarshal(docs.OpenAPISpec, &spec); err != nil {
			h.logger.Error("Failed to parse embedded OpenAPI spec", "error", err)
			return
		}
		converted := convertYAMLToJSON(spec)
		data, err := json.Marshal(converted)
		if err != nil {
			h.logger.Error("Failed to marshal OpenAPI spec to JSON", "error", err)
			return
		}
		cachedJSON = data
	})

	if cachedJSON == nil {
		h.writeError(w, http.StatusInternalServerError, "spec_error", "Failed to load OpenAPI spec")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(cachedJSON)
}

// convertYAMLToJSON recursively converts YAML-decoded maps (map[string]interface{})
// from the yaml.v3 decoder which uses map[string]interface{} by default.
func convertYAMLToJSON(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(val))
		for k, v := range val {
			result[k] = convertYAMLToJSON(v)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, v := range val {
			result[i] = convertYAMLToJSON(v)
		}
		return result
	default:
		return v
	}
}
