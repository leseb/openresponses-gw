// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package vectorstore

// DefaultChunkSize is the default chunk size in characters.
const DefaultChunkSize = 800

// DefaultChunkOverlap is the default overlap between chunks in characters.
const DefaultChunkOverlap = 200

// ChunkText splits text into fixed-size chunks with configurable overlap.
// chunkSize and overlap are in characters. If chunkSize <= 0, DefaultChunkSize is used.
// If overlap < 0 or >= chunkSize, DefaultChunkOverlap is used (clamped to < chunkSize).
func ChunkText(text string, chunkSize, overlap int) []string {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	if overlap < 0 || overlap >= chunkSize {
		overlap = DefaultChunkOverlap
		if overlap >= chunkSize {
			overlap = chunkSize / 4
		}
	}

	if len(text) == 0 {
		return nil
	}

	var chunks []string
	step := chunkSize - overlap
	if step <= 0 {
		step = 1
	}

	for start := 0; start < len(text); start += step {
		end := start + chunkSize
		if end > len(text) {
			end = len(text)
		}
		chunks = append(chunks, text[start:end])
		if end == len(text) {
			break
		}
	}

	return chunks
}

// TokensToChars converts a token count to an approximate character count
// using a ~4:1 ratio (chars per token).
func TokensToChars(tokens int) int {
	return tokens * 4
}
