// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package extractor

// extractText returns the content as-is (plain text pass-through).
func extractText(content []byte) (string, error) {
	return string(content), nil
}
