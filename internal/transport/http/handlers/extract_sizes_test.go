package handlers

import "testing"

func TestShouldEnrichPlatform_IncludesAriaExtendedCommonPlatforms(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		want     bool
	}{
		{name: "youtube", platform: "youtube", want: true},
		{name: "generic", platform: "generic", want: true},
		{name: "trim and case normalization", platform: "  YouTube ", want: true},
		{name: "unsupported", platform: "example", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldEnrichPlatform(tt.platform); got != tt.want {
				t.Fatalf("shouldEnrichPlatform(%q) = %v, want %v", tt.platform, got, tt.want)
			}
		})
	}
}
