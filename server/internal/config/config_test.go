package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAllowedOrigins_Default(t *testing.T) {
	t.Setenv("BACKUP_ALLOWED_ORIGINS", "")

	got := getAllowedOrigins()
	assert.Equal(t, defaultAllowedOrigins, got)
}

func TestGetAllowedOrigins_Single(t *testing.T) {
	t.Setenv("BACKUP_ALLOWED_ORIGINS", "https://app.example.com")

	got := getAllowedOrigins()
	assert.Equal(t, []string{"https://app.example.com"}, got)
}

func TestGetAllowedOrigins_Multiple(t *testing.T) {
	t.Setenv("BACKUP_ALLOWED_ORIGINS", "https://app.example.com,https://admin.example.com")

	got := getAllowedOrigins()
	assert.Equal(t, []string{"https://app.example.com", "https://admin.example.com"}, got)
}

func TestGetAllowedOrigins_TrimsWhitespace(t *testing.T) {
	t.Setenv("BACKUP_ALLOWED_ORIGINS", " https://app.example.com , https://admin.example.com ")

	got := getAllowedOrigins()
	assert.Equal(t, []string{"https://app.example.com", "https://admin.example.com"}, got)
}

func TestGetAllowedOrigins_EmptyEntriesIgnored(t *testing.T) {
	t.Setenv("BACKUP_ALLOWED_ORIGINS", "https://app.example.com,,https://admin.example.com")

	got := getAllowedOrigins()
	assert.Equal(t, []string{"https://app.example.com", "https://admin.example.com"}, got)
}

func TestGetAllowedOrigins_AllWhitespaceFallsBackToDefault(t *testing.T) {
	t.Setenv("BACKUP_ALLOWED_ORIGINS", "  ,  ")

	got := getAllowedOrigins()
	assert.Equal(t, defaultAllowedOrigins, got)
}
