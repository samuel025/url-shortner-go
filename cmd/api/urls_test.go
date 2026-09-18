package main

import (
	"strings"
	"testing"
)

func TestIsValidURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"Valid HTTP", "http://example.com", true},
		{"Valid HTTPS with path", "https://github.com/samuel025/DRMQ", true},
		{"Valid HTTPS with query", "https://example.com/search?q=golang&page=1", true},
		{"Empty URL", "", false},
		{"FTP scheme", "ftp://example.com/file", false},
		{"Javascript URI", "javascript:alert(1)", false},
		{"Missing host", "https://", false},
		{"Relative path", "/path/to/resource", false},
		{"Excessively long URL", "https://example.com/" + strings.Repeat("a", 2050), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidURL(tt.url); got != tt.expected {
				t.Errorf("isValidURL(%q) = %v, expected %v", tt.url, got, tt.expected)
			}
		})
	}
}

func TestGenerateShortCode(t *testing.T) {
	length := 6
	code, err := generateShortCode(length)
	if err != nil {
		t.Fatalf("unexpected error generating code: %v", err)
	}

	if len(code) != length {
		t.Fatalf("expected length %d, got %d", length, len(code))
	}

	for _, char := range code {
		if !strings.ContainsRune(shortCodeCharset, char) {
			t.Errorf("character %c not in allowed charset", char)
		}
	}
}
