// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"path/filepath"
	"strings"
)

// ExtractText extracts plain text from file content based on the file extension.
// Falls back to treating content as plain text for unsupported formats.
func ExtractText(content []byte, filename string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".pdf":
		return extractPDF(content)
	case ".html", ".htm":
		return extractHTML(content)
	case ".csv":
		return extractCSV(content)
	case ".json":
		return extractJSON(content)
	case ".jsonl":
		return extractJSONL(content)
	default:
		return extractText(content)
	}
}
