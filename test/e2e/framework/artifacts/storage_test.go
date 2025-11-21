package artifacts

import (
	"bytes"
	"testing"
)

func TestTruncateLogLines_RemovesLeadingWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLines int
		want     string
	}{
		{
			name:     "removes leading newlines and spaces",
			input:    "\n\n  \t2025-11-14T19:58:40Z\tINFO\tstart Reconcile\nline2\nline3",
			maxLines: 3,
			want:     "... [Showing last 3 lines] ...\n2025-11-14T19:58:40Z\tINFO\tstart Reconcile\nline2\nline3",
		},
		{
			name:     "handles logs without leading whitespace",
			input:    "line1\nline2\nline3\nline4\nline5",
			maxLines: 3,
			want:     "... [Showing last 3 lines] ...\nline3\nline4\nline5",
		},
		{
			name:     "keeps content when less than maxLines",
			input:    "line1\nline2",
			maxLines: 5,
			want:     "line1\nline2",
		},
		{
			name:     "trims leading whitespace from first line but preserves it in subsequent lines",
			input:    "line1\nline2\nline3 with content\n  indented line4\n  indented line5",
			maxLines: 3,
			want:     "... [Showing last 3 lines] ...\nline3 with content\n  indented line4\n  indented line5",
		},
		{
			name:     "handles real operator log format",
			input:    "line1\nline2\nline3\nline4\n2025-11-14T19:58:40Z\tINFO\tstart Reconcile",
			maxLines: 1,
			want:     "... [Showing last 1 lines] ...\n2025-11-14T19:58:40Z\tINFO\tstart Reconcile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateLogLines([]byte(tt.input), tt.maxLines)
			if !bytes.Equal(got, []byte(tt.want)) {
				t.Errorf("TruncateLogLines() = %q, want %q", string(got), tt.want)
			}
		})
	}
}
