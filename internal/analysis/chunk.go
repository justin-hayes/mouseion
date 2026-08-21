package analysis

// DefaultMaxChunkChars bounds each Analyze RPC (ADR 0013). Empirical testing
// showed ~100k chars of German text analyzes in ~54s and yields a ~2 MiB gRPC
// response, both comfortably within limits. Chunks larger than this can exceed
// the call deadline or the gRPC 4 MiB message limit.
const DefaultMaxChunkChars = 100_000

// splitSentences splits text into sentences at sentence-ending punctuation.
// It is a lightweight splitter used only to keep chunk boundaries sentence
// aligned; it does not need to be perfect.
func splitSentences(text string) []string {
	var (
		sentences []string
		start     int
	)
	for i, r := range text {
		if r == '.' || r == '!' || r == '?' {
			end := i + len(string(r))
			sentences = append(sentences, text[start:end])
			start = end
		}
	}
	if start < len(text) {
		sentences = append(sentences, text[start:])
	}
	return sentences
}

// chunkText splits text into chunks of at most maxChars, preferring to split
// at sentence boundaries. If a single sentence exceeds maxChars (e.g. an
// extremely long sentence or minified text with no sentence punctuation), it is
// hard-cut to keep every chunk within the bound.
func chunkText(text string, maxChars int) []string {
	if maxChars <= 0 {
		maxChars = DefaultMaxChunkChars
	}
	if len([]rune(text)) <= maxChars {
		return []string{text}
	}
	sentences := splitSentences(text)
	var chunks []string
	var current []rune
	flush := func() {
		if len(current) > 0 {
			chunks = append(chunks, string(current))
			current = nil
		}
	}
	for _, sentence := range sentences {
		remainder := []rune(sentence)
		// Hard-cut an over-long sentence so it still fits a chunk.
		for len(remainder) > maxChars {
			flush()
			chunks = append(chunks, string(remainder[:maxChars]))
			remainder = remainder[maxChars:]
		}
		if len(current)+len(remainder) > maxChars {
			flush()
		}
		current = append(current, remainder...)
	}
	flush()
	if len(chunks) == 0 {
		return []string{text}
	}
	return chunks
}
