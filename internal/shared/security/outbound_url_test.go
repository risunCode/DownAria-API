package security

import (
	"context"
	"net"
	"testing"
)

type stubResolver struct {
	lookup func(ctx context.Context, host string) ([]net.IPAddr, error)
}

func (s stubResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return s.lookup(ctx, host)
}

func TestOutboundURLValidator_Validate(t *testing.T) {
	resolver := stubResolver{lookup: func(ctx context.Context, host string) ([]net.IPAddr, error) {
		switch host {
		case "example.com":
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		case "dual.example":
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("10.0.0.1")}}, nil
		default:
			return nil, &net.DNSError{Err: "not found", Name: host}
		}
	}}

	v := NewOutboundURLValidator(resolver)

	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{name: "allow public https host", rawURL: "https://example.com/video.mp4", wantErr: false},
		{name: "allow public http host", rawURL: "http://example.com/video.mp4", wantErr: false},
		{name: "block unsupported scheme", rawURL: "ftp://example.com/file", wantErr: true},
		{name: "block localhost host", rawURL: "https://localhost/admin", wantErr: true},
		{name: "block localhost suffix", rawURL: "https://api.localhost/admin", wantErr: true},
		{name: "block loopback ip", rawURL: "http://127.0.0.1/status", wantErr: true},
		{name: "block private ip", rawURL: "http://10.1.2.3/status", wantErr: true},
		{name: "block link-local ip", rawURL: "http://169.254.1.10/meta", wantErr: true},
		{name: "block multicast ip", rawURL: "http://224.0.0.1/mcast", wantErr: true},
		{name: "block unspecified ipv6", rawURL: "http://[::]/x", wantErr: true},
		{name: "block loopback ipv6", rawURL: "http://[::1]/x", wantErr: true},
		{name: "block when dns answer includes private ip", rawURL: "https://dual.example/path", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := v.Validate(context.Background(), tt.rawURL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate(%q) err=%v wantErr=%v", tt.rawURL, err, tt.wantErr)
			}
		})
	}
}
