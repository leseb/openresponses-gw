// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"bytes"
	"encoding/json"
	"strings"
)

// extractJSON pretty-prints a JSON document for text extraction.
func extractJSON(content []byte) (string, error) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, content, "", "  "); err != nil {
		// If not valid JSON, return as-is
		return string(content), nil
	}
	return buf.String(), nil
}

// extractJSONL processes a JSONL file (one JSON object per line),
// pretty-printing each line and separating with blank lines.
func extractJSONL(content []byte) (string, error) {
	lines := strings.Split(string(content), "\n")
	var sb strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(line), "", "  "); err != nil {
			// Not valid JSON line, include as-is
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(line)
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(buf.String())
	}
	return sb.String(), nil
}
