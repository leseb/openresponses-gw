// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"strings"
	"testing"
)

func TestExtractText(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  []byte
		contains string // substring the result should contain
		wantErr  bool
	}{
		{
			name:     "plain text passthrough",
			filename: "readme.txt",
			content:  []byte("Hello, world!"),
			contains: "Hello, world!",
		},
		{
			name:     "unknown extension treated as text",
			filename: "data.xyz",
			content:  []byte("raw content"),
			contains: "raw content",
		},
		{
			name:     "HTML extraction",
			filename: "page.html",
			content:  []byte("<html><body><p>Hello</p><script>var x=1;</script><p>World</p></body></html>"),
			contains: "Hello",
		},
		{
			name:     "HTML skips script",
			filename: "page.htm",
			content:  []byte("<html><script>alert('x')</script><body>visible</body></html>"),
			contains: "visible",
		},
		{
			name:     "CSV extraction",
			filename: "data.csv",
			content:  []byte("name,age,city\nAlice,30,NYC\nBob,25,LA"),
			contains: "Alice",
		},
		{
			name:     "JSON pretty-print",
			filename: "config.json",
			content:  []byte(`{"key":"value","num":42}`),
			contains: "\"key\": \"value\"",
		},
		{
			name:     "JSONL extraction",
			filename: "logs.jsonl",
			content:  []byte("{\"a\":1}\n{\"b\":2}"),
			contains: "\"a\": 1",
		},
		{
			name:     "invalid JSON falls back to raw",
			filename: "bad.json",
			content:  []byte("not json at all"),
			contains: "not json at all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractText(tt.content, tt.filename)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ExtractText() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !strings.Contains(result, tt.contains) {
				t.Errorf("ExtractText() = %q, want substring %q", result, tt.contains)
			}
		})
	}
}

func TestExtractHTML_NoScript(t *testing.T) {
	content := []byte("<html><head><style>body{}</style></head><body><p>Content here</p></body></html>")
	result, err := ExtractText(content, "test.html")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result, "body{}") {
		t.Error("HTML extraction should strip style content")
	}
	if !strings.Contains(result, "Content here") {
		t.Errorf("expected 'Content here' in result, got %q", result)
	}
}

func TestExtractCSV_TabSeparated(t *testing.T) {
	content := []byte("a,b,c\n1,2,3")
	result, err := ExtractText(content, "data.csv")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "a\tb\tc" {
		t.Errorf("expected tab-separated header, got %q", lines[0])
	}
}
