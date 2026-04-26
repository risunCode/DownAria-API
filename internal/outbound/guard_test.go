package outbound

import (
	"context"
	"net"
	"testing"
)

type staticResolver map[string][]net.IPAddr

func (r staticResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return r[host], nil
}

func TestGuardBlocksPrivateAndAllowsPublic(t *testing.T) {
	guard, err := NewGuard(staticResolver{
		"private.example": {{IP: net.ParseIP("127.0.0.1")}},
		"public.example":  {{IP: net.ParseIP("93.184.216.34")}},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := guard.Validate(context.Background(), "https://private.example/a"); err == nil {
		t.Fatal("expected private block")
	}
	if _, err := guard.Validate(context.Background(), "https://public.example/a"); err != nil {
		t.Fatalf("public blocked: %v", err)
	}
}

func TestSanitizeHTTPURLRejectsInvalidScheme(t *testing.T) {
	if _, err := SanitizeHTTPURL("file:///etc/passwd"); err == nil {
		t.Fatal("expected invalid scheme")
	}
}
