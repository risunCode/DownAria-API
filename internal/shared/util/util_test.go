package util

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestClientIPFromRequest(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		want       string
	}{
		{
			name:       "X-Forwarded-For ignored without trusted proxy",
			headers:    map[string]string{"X-Forwarded-For": "192.168.1.1"},
			remoteAddr: "10.0.0.1:1234",
			want:       "10.0.0.1",
		},
		{
			name:       "X-Forwarded-For multiple IPs ignored without trusted proxy",
			headers:    map[string]string{"X-Forwarded-For": "192.168.1.1, 10.0.0.2, 172.16.0.1"},
			remoteAddr: "10.0.0.1:1234",
			want:       "10.0.0.1",
		},
		{
			name:       "X-Real-IP ignored without trusted proxy",
			headers:    map[string]string{"X-Real-IP": "10.0.0.5"},
			remoteAddr: "10.0.0.1:1234",
			want:       "10.0.0.1",
		},
		{
			name:       "X-Forwarded-For does not override without trusted proxy",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.1", "X-Real-IP": "10.0.0.5"},
			remoteAddr: "10.0.0.1:1234",
			want:       "10.0.0.1",
		},
		{
			name:       "RemoteAddr fallback",
			headers:    map[string]string{},
			remoteAddr: "10.0.0.1:1234",
			want:       "10.0.0.1",
		},
		{
			name:       "nil request",
			headers:    nil,
			remoteAddr: "",
			want:       "",
		},
		{
			name:       "X-Forwarded-For with whitespace",
			headers:    map[string]string{"X-Forwarded-For": "  192.168.1.1  "},
			remoteAddr: "10.0.0.1:1234",
			want:       "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.headers != nil {
				req = httptest.NewRequest("GET", "/", nil)
				for k, v := range tt.headers {
					req.Header.Set(k, v)
				}
				if tt.remoteAddr != "" {
					req.RemoteAddr = tt.remoteAddr
				}
			}

			got := ClientIPFromRequest(req)
			if got != tt.want {
				t.Errorf("ClientIPFromRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClientIPFromRequestWithTrustedProxies(t *testing.T) {
	trusted, err := NewIPAllowlist([]string{"10.0.0.0/8", "192.168.1.10"})
	if err != nil {
		t.Fatalf("failed to build allowlist: %v", err)
	}

	t.Run("trust x-forwarded-for from trusted proxy", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.1.2.3:1234"
		req.Header.Set("X-Forwarded-For", "198.51.100.20, 10.0.0.2")

		got := ClientIPFromRequestWithTrustedProxies(req, trusted)
		if got != "198.51.100.20" {
			t.Fatalf("expected forwarded ip, got %s", got)
		}
	})

	t.Run("fall back to x-real-ip from trusted proxy", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.1.2.3:1234"
		req.Header.Set("X-Real-IP", "203.0.113.77")

		got := ClientIPFromRequestWithTrustedProxies(req, trusted)
		if got != "203.0.113.77" {
			t.Fatalf("expected X-Real-IP, got %s", got)
		}
	})

	t.Run("ignore spoofed headers from untrusted proxy", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "203.0.113.10:1234"
		req.Header.Set("X-Forwarded-For", "198.51.100.20")

		got := ClientIPFromRequestWithTrustedProxies(req, trusted)
		if got != "203.0.113.10" {
			t.Fatalf("expected remote addr ip, got %s", got)
		}
	})
}

func TestDefaultUserAgent(t *testing.T) {
	if DefaultUserAgent == "" {
		t.Error("DefaultUserAgent should not be empty")
	}
	if len(DefaultUserAgent) < 10 {
		t.Error("DefaultUserAgent should be a valid browser user agent string")
	}
}

func TestParseInt64OrZero(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int64
	}{
		{"valid positive number", "123", 123},
		{"valid negative number", "-456", -456},
		{"valid zero", "0", 0},
		{"valid large number", "9223372036854775807", 9223372036854775807},
		{"invalid string", "abc", 0},
		{"empty string", "", 0},
		{"whitespace only", "   ", 0},
		{"mixed invalid", "123abc", 0},
		{"whitespace trimmed", "  456  ", 456},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseInt64OrZero(tt.value)
			if got != tt.want {
				t.Errorf("ParseInt64OrZero(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestParseIntOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		fallback int
		want     int
	}{
		{"valid number", "42", 100, 42},
		{"invalid string", "notanumber", 100, 100},
		{"empty string", "", 100, 100},
		{"whitespace only", "   ", 50, 50},
		{"whitespace trimmed", "  25  ", 100, 25},
		{"negative number", "-10", 100, -10},
		{"zero", "0", 100, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseIntOrDefault(tt.value, tt.fallback)
			if got != tt.want {
				t.Errorf("ParseIntOrDefault(%q, %d) = %v, want %v", tt.value, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestExtractLeadingDigitsIntOrZero(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int
	}{
		{"digits at start", "123abc", 123},
		{"digits in middle", "abc456def", 0},
		{"digits at end", "abc789", 0},
		{"only digits", "456", 456},
		{"empty string", "", 0},
		{"no digits", "abcdef", 0},
		{"whitespace then digits", "  789xyz", 789},
		{"single digit", "5test", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractLeadingDigitsIntOrZero(tt.value)
			if got != tt.want {
				t.Errorf("ExtractLeadingDigitsIntOrZero(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestClampNonNegativeInt64(t *testing.T) {
	tests := []struct {
		name  string
		value int64
		want  int64
	}{
		{"negative returns zero", -10, 0},
		{"zero stays zero", 0, 0},
		{"positive unchanged", 50, 50},
		{"large positive unchanged", 9223372036854775807, 9223372036854775807},
		{"small negative returns zero", -1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClampNonNegativeInt64(tt.value)
			if got != tt.want {
				t.Errorf("ClampNonNegativeInt64(%v) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestExtractFirstRegexGroup(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		pattern string
		want    string
	}{
		{"match found", "hello world", `hello (\w+)`, "world"},
		{"no match", "goodbye world", `hello (\w+)`, ""},
		{"multiple groups takes first", "hello world foo", `hello (\w+) (\w+)`, "world"},
		{"no groups in pattern", "hello world", `hello \w+`, ""},
		{"empty string", "", `hello (\w+)`, ""},
		{"empty value with empty match", "", `()`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			got := ExtractFirstRegexGroup(tt.value, re)
			if got != tt.want {
				t.Errorf("ExtractFirstRegexGroup(%q, %q) = %v, want %v", tt.value, tt.pattern, got, tt.want)
			}
		})
	}

	t.Run("nil regex returns empty", func(t *testing.T) {
		got := ExtractFirstRegexGroup("test", nil)
		if got != "" {
			t.Errorf("ExtractFirstRegexGroup with nil regex = %v, want empty string", got)
		}
	})
}

func TestGenerateRequestID(t *testing.T) {
	t.Run("uniqueness", func(t *testing.T) {
		ids := make(map[string]bool)
		for i := 0; i < 100; i++ {
			id := GenerateRequestID()
			if ids[id] {
				t.Errorf("GenerateRequestID() returned duplicate ID: %s", id)
			}
			ids[id] = true
		}
	})

	t.Run("format and length", func(t *testing.T) {
		id := GenerateRequestID()

		// Should be hex string of 24 bytes (12 bytes * 2 hex chars per byte)
		if len(id) != 24 {
			t.Errorf("GenerateRequestID() length = %d, want 24", len(id))
		}

		// Should only contain hex characters
		validHex := regexp.MustCompile(`^[0-9a-f]+$`)
		if !validHex.MatchString(id) {
			t.Errorf("GenerateRequestID() = %s, want valid hex string", id)
		}
	})
}
