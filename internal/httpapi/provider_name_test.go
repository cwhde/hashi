package httpapi

import (
	"context"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
)

// Bug 114: a long provider name broke the rule rows; the server refuses it before reading the provider.
func TestSaveProviderRefusesLongName(t *testing.T) {
	s := &Server{}
	in := gen.SSOProviderInput{Name: strings.Repeat("x", maxProviderName+1), Issuer: "https://sso.example.com", ClientId: "c"}
	if _, err := s.saveProvider(context.Background(), "", in); err == nil || !strings.Contains(err.Error(), "32 characters") {
		t.Fatalf("want the length refused, got %v", err)
	}
}
