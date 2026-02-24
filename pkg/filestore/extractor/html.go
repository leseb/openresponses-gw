// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// extractHTML strips HTML tags and returns the visible text content.
// Script and style elements are skipped entirely.
func extractHTML(content []byte) (string, error) {
	doc, err := html.Parse(bytes.NewReader(content))
	if err != nil {
		// Fall back to raw text if HTML is malformed
		return string(content), nil
	}

	var sb strings.Builder
	extractTextFromNode(doc, &sb)
	return strings.TrimSpace(sb.String()), nil
}

func extractTextFromNode(n *html.Node, sb *strings.Builder) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "script", "style", "noscript":
			return
		}
	}

	if n.Type == html.TextNode {
		text := strings.TrimSpace(n.Data)
		if text != "" {
			if sb.Len() > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(text)
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractTextFromNode(c, sb)
	}
}
