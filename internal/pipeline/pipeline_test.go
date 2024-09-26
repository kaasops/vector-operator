package pipeline

import (
	"testing"
)

func TestMatchLabels(t *testing.T) {
	tests := []struct {
		name     string
		selector map[string]string
		labels   map[string]string
		want     bool
	}{
		{
			name:     "NoSelector",
			selector: nil,
			labels:   map[string]string{"label1": "value1", "label2": "value2"},
			want:     true,
		},
		{
			name:     "MatchingLabels",
			selector: map[string]string{"label1": "value1", "label2": "value2"},
			labels:   map[string]string{"label1": "value1", "label2": "value2"},
			want:     true,
		},
		{
			name:     "MismatchedLabelValues",
			selector: map[string]string{"label1": "value1", "label2": "value2"},
			labels:   map[string]string{"label1": "value1", "label2": "mismatch"},
			want:     false,
		},
		{
			name:     "ExtraLabelsInMap",
			selector: map[string]string{"label1": "value1"},
			labels:   map[string]string{"label1": "value1", "label2": "value2"},
			want:     true,
		},
		{
			name:     "SelectorWithNoMatches",
			selector: map[string]string{"label1": "value1", "label2": "value2"},
			labels:   map[string]string{"label3": "value3"},
			want:     false,
		},
		{
			name:     "SelectorWithNoMatches2",
			selector: map[string]string{"label1": "value1", "label2": "value2"},
			labels:   map[string]string{"label1": "label1"},
			want:     false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := MatchLabels(test.selector, test.labels); got != test.want {
				t.Errorf("MatchLabels() = %v, want %v", got, test.want)
			}
		})
	}
}
