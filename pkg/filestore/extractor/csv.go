// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"bytes"
	"encoding/csv"
	"io"
	"strings"
)

// extractCSV reads CSV content and returns it as tab-separated text.
// Each row is joined with tabs, rows are separated by newlines.
func extractCSV(content []byte) (string, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1 // allow variable field counts

	var sb strings.Builder
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// If CSV parsing fails, fall back to raw text
			return string(content), nil
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(strings.Join(record, "\t"))
	}

	return sb.String(), nil
}
