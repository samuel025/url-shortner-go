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
		{"Valid HTTPS with path", "https://github.com/samuel025/url-shortner-go", true},
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

func TestIsValidShortCode(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{"Valid 6-char code", "a8Fk2A", true},
		{"Valid single char", "x", true},
		{"Valid 16-char code", "abcdefghijklmnop", true},
		{"Empty code", "", false},
		{"Too long code (>16)", "abcdefghijklmnopq", false},
		{"Invalid characters (hyphen)", "a8-Fk2", false},
		{"Invalid characters (special)", "a8$Fk2", false},
		{"Whitespace", "a8 Fk2", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidShortCode(tt.code); got != tt.expected {
				t.Errorf("isValidShortCode(%q) = %v, expected %v", tt.code, got, tt.expected)
			}
		})
	}
}
