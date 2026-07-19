package web

import "testing"

func TestNormalizeRegistrarName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"GoDaddy.com, LLC", "GoDaddy.com, LLC"},
		{"  Gandi SAS  ", "Gandi SAS"},
		{"Cloudflare, Inc. [Tag = CLOUDFLARE]", "Cloudflare, Inc."},
		{"Gandi [Tag = GANDI]", "Gandi"},
		{"20i Ltd [Tag = STACK]", "20i Ltd"},
		{"", ""},
		{"[Tag = ORPHAN]", ""},
	}
	for _, tt := range tests {
		if got := normalizeRegistrarName(tt.in); got != tt.want {
			t.Errorf("normalizeRegistrarName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRegistrarGroupKey(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Cloudflare, Inc.", "cloudflare, inc"},
		{"Cloudflare, Inc", "cloudflare, inc"},
		{"GANDI SAS", "gandi sas"},
		{"Gandi SAS", "gandi sas"},
		{"IONOS SE", "ionos se"},
		{".,", ""},
	}
	for _, tt := range tests {
		if got := registrarGroupKey(tt.in); got != tt.want {
			t.Errorf("registrarGroupKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
