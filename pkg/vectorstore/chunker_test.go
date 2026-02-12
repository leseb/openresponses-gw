// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package vectorstore

import (
	"strings"
	"testing"
)

func TestChunkText_EmptyInput(t *testing.T) {
	result := ChunkText("", 100, 10)
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestChunkText_ShortText(t *testing.T) {
	text := "hello"
	chunks := ChunkText(text, 100, 10)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("expected %q, got %q", text, chunks[0])
	}
}

func TestChunkText_ExactChunkSize(t *testing.T) {
	text := "abcde"
	chunks := ChunkText(text, 5, 0)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("expected %q, got %q", text, chunks[0])
	}
}

func TestChunkText_BasicChunking(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		chunkSize int
		overlap   int
		wantCount int
	}{
		{
			name:      "no overlap",
			text:      "abcdefghij",
			chunkSize: 5,
			overlap:   0,
			wantCount: 2,
		},
		{
			name:      "with overlap",
			text:      "abcdefghij",
			chunkSize: 5,
			overlap:   2,
			wantCount: 3,
		},
		{
			name:      "large overlap",
			text:      "abcdefghij",
			chunkSize: 5,
			overlap:   4,
			wantCount: 6, // step=1, starts at 0,1,2,3,4,5 then end==len
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := ChunkText(tt.text, tt.chunkSize, tt.overlap)
			if len(chunks) != tt.wantCount {
				t.Errorf("expected %d chunks, got %d: %v", tt.wantCount, len(chunks), chunks)
			}
			// Verify each chunk is at most chunkSize
			for i, c := range chunks {
				if len(c) > tt.chunkSize {
					t.Errorf("chunk[%d] length %d exceeds chunkSize %d", i, len(c), tt.chunkSize)
				}
			}
			// Verify overlap content matches between adjacent chunks
			if tt.overlap > 0 && len(chunks) > 1 {
				for i := 0; i < len(chunks)-1; i++ {
					step := tt.chunkSize - tt.overlap
					if step <= 0 {
						step = 1
					}
					// The suffix of chunk[i] starting at position step should equal
					// the prefix of chunk[i+1] of the same length
					suffixLen := len(chunks[i]) - step
					if suffixLen > 0 && suffixLen <= len(chunks[i+1]) {
						suffix := chunks[i][step:]
						prefix := chunks[i+1][:suffixLen]
						if suffix != prefix {
							t.Errorf("overlap mismatch between chunk[%d] and chunk[%d]: %q vs %q",
								i, i+1, suffix, prefix)
						}
					}
				}
			}
		})
	}
}

func TestChunkText_DefaultChunkSize(t *testing.T) {
	tests := []struct {
		name      string
		chunkSize int
	}{
		{"zero", 0},
		{"negative", -1},
	}

	// Text shorter than DefaultChunkSize -> 1 chunk
	text := strings.Repeat("x", 500)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := ChunkText(text, tt.chunkSize, 0)
			if len(chunks) != 1 {
				t.Errorf("expected 1 chunk with default chunk size (%d), got %d", DefaultChunkSize, len(chunks))
			}
		})
	}

	// Text longer than DefaultChunkSize -> multiple chunks
	longText := strings.Repeat("y", DefaultChunkSize+100)
	chunks := ChunkText(longText, 0, 0)
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks for text longer than DefaultChunkSize, got %d", len(chunks))
	}
}

func TestChunkText_OverlapClamping(t *testing.T) {
	text := strings.Repeat("a", 100)
	tests := []struct {
		name    string
		overlap int
	}{
		{"negative overlap", -5},
		{"overlap equals chunkSize", 20},
		{"overlap exceeds chunkSize", 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := ChunkText(text, 20, tt.overlap)
			// Should not panic and should produce valid chunks
			if len(chunks) == 0 {
				t.Error("expected at least one chunk")
			}
			// With clamped overlap (chunkSize/4 = 5), step = 20-5 = 15
			// So we expect ceil(100/15) chunks
			expectedCount := 7 // 100 / 15 = 6.67, so 7 chunks
			if len(chunks) != expectedCount {
				t.Errorf("expected %d chunks with clamped overlap, got %d", expectedCount, len(chunks))
			}
		})
	}
}

func TestChunkText_MinimalChunkSize(t *testing.T) {
	text := "abc"
	chunks := ChunkText(text, 1, 0)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks for chunkSize=1, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) != 1 {
			t.Errorf("chunk[%d] expected length 1, got %d", i, len(c))
		}
	}
	if chunks[0] != "a" || chunks[1] != "b" || chunks[2] != "c" {
		t.Errorf("unexpected chunks: %v", chunks)
	}
}

func TestChunkText_LargeText(t *testing.T) {
	text := strings.Repeat("x", 2000)
	chunks := ChunkText(text, DefaultChunkSize, DefaultChunkOverlap)

	// With chunkSize=800, overlap=200, step=600
	// start=0: [0,800), start=600: [600,1400), start=1200: [1200,2000) -> end==len, break
	expectedCount := 3
	if len(chunks) != expectedCount {
		t.Errorf("expected %d chunks, got %d", expectedCount, len(chunks))
	}

	// First chunk should be full size
	if len(chunks[0]) != DefaultChunkSize {
		t.Errorf("first chunk expected %d chars, got %d", DefaultChunkSize, len(chunks[0]))
	}

	// Last chunk is also full size here (1200+800=2000==len)
	lastChunk := chunks[len(chunks)-1]
	if len(lastChunk) != DefaultChunkSize {
		t.Errorf("last chunk expected %d chars, got %d", DefaultChunkSize, len(lastChunk))
	}

	// Verify overlap between first two chunks
	step := DefaultChunkSize - DefaultChunkOverlap
	overlapRegion1 := chunks[0][step:]
	overlapRegion2 := chunks[1][:DefaultChunkOverlap]
	if overlapRegion1 != overlapRegion2 {
		t.Errorf("overlap region mismatch between chunk 0 and 1")
	}
}

func TestTokensToChars(t *testing.T) {
	tests := []struct {
		tokens int
		want   int
	}{
		{100, 400},
		{0, 0},
		{1, 4},
	}
	for _, tt := range tests {
		got := TokensToChars(tt.tokens)
		if got != tt.want {
			t.Errorf("TokensToChars(%d) = %d, want %d", tt.tokens, got, tt.want)
		}
	}
}
