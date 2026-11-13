// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func oidcEnv() map[string]string {
	return map[string]string{
		"CHRYSALIS_BASE_URL":           "https://board.example.home/",
		"CHRYSALIS_OIDC_ISSUER":        "https://auth.example.com",
		"CHRYSALIS_OIDC_CLIENT_ID":     "chrysalis",
		"CHRYSALIS_OIDC_CLIENT_SECRET": "not-a-real-secret",
	}
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(envFrom(oidcEnv()))
	if err != nil {
		t.Fatal(err)
	}
	if c.AuthMode != AuthOIDC || c.ListenAddr != ":8080" || c.DataDir != "/data" || c.SessionTTL != 720*time.Hour || c.TrashRetention != 30*24*time.Hour || c.MaxUploadBytes != 25<<20 || c.StorageBackend != "local" {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if c.Backup != (Backup{Dir: "/data/backups", KeepDaily: 7, KeepWeekly: 4, Schedule: "off"}) {
		t.Errorf("Backup = %+v", c.Backup)
	}
	if got := c.BaseURL.String(); got != "https://board.example.home" {
		t.Errorf("BaseURL = %q, want trailing slash trimmed", got)
	}
	if c.OIDC.RedirectURL != "https://board.example.home/auth/callback" {
		t.Errorf("RedirectURL = %q", c.OIDC.RedirectURL)
	}
	if strings.Join(c.OIDC.Scopes, " ") != "openid profile email" {
		t.Errorf("Scopes = %v", c.OIDC.Scopes)
	}
	if c.Header.User != "Remote-User" || !c.SecureCookies() {
		t.Errorf("header names or secure cookies wrong: %+v", c)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"missing base url", map[string]string{"CHRYSALIS_AUTH_MODE": "dev", "CHRYSALIS_DEV_MODE": "true"}, "CHRYSALIS_BASE_URL is required"},
		{"relative base url", map[string]string{"CHRYSALIS_BASE_URL": "/board", "CHRYSALIS_AUTH_MODE": "dev", "CHRYSALIS_DEV_MODE": "true"}, "absolute http(s) URL"},
		{"oidc without issuer", map[string]string{"CHRYSALIS_BASE_URL": "https://board.example.home"}, "oidc mode requires"},
		{"header without cidrs", map[string]string{"CHRYSALIS_BASE_URL": "https://board.example.home", "CHRYSALIS_AUTH_MODE": "header"}, "header mode requires CHRYSALIS_TRUSTED_PROXY_CIDRS"},
		{"bad cidr", map[string]string{"CHRYSALIS_BASE_URL": "https://board.example.home", "CHRYSALIS_AUTH_MODE": "header", "CHRYSALIS_TRUSTED_PROXY_CIDRS": "10.0.0.0/8,nope"}, `invalid entry "nope"`},
		{"dev without dev mode", map[string]string{"CHRYSALIS_BASE_URL": "http://localhost:8080", "CHRYSALIS_AUTH_MODE": "dev"}, "CHRYSALIS_DEV_MODE=true"},
		{"unknown mode", map[string]string{"CHRYSALIS_BASE_URL": "http://localhost:8080", "CHRYSALIS_AUTH_MODE": "magic"}, "must be oidc, header or dev"},
		{"bad trash retention", map[string]string{"CHRYSALIS_BASE_URL": "http://localhost:8080", "CHRYSALIS_AUTH_MODE": "dev", "CHRYSALIS_DEV_MODE": "true", "CHRYSALIS_TRASH_RETENTION_DAYS": "0"}, "TRASH_RETENTION_DAYS"},
		{"upload limit zero", map[string]string{"CHRYSALIS_BASE_URL": "http://localhost:8080", "CHRYSALIS_AUTH_MODE": "dev", "CHRYSALIS_DEV_MODE": "true", "CHRYSALIS_MAX_UPLOAD_MB": "0"}, "MAX_UPLOAD_MB"},
		{"s3 not yet", map[string]string{"CHRYSALIS_BASE_URL": "http://localhost:8080", "CHRYSALIS_AUTH_MODE": "dev", "CHRYSALIS_DEV_MODE": "true", "CHRYSALIS_STORAGE_BACKEND": "s3"}, "isn't available yet"},
		{"bad backup schedule", map[string]string{"CHRYSALIS_BASE_URL": "http://localhost:8080", "CHRYSALIS_AUTH_MODE": "dev", "CHRYSALIS_DEV_MODE": "true", "CHRYSALIS_BACKUP_SCHEDULE": "hourly"}, "BACKUP_SCHEDULE"},
		{"no daily backups", map[string]string{"CHRYSALIS_BASE_URL": "http://localhost:8080", "CHRYSALIS_AUTH_MODE": "dev", "CHRYSALIS_DEV_MODE": "true", "CHRYSALIS_BACKUP_KEEP_DAILY": "0"}, "BACKUP_KEEP_DAILY"},
		{"negative weekly backups", map[string]string{"CHRYSALIS_BASE_URL": "http://localhost:8080", "CHRYSALIS_AUTH_MODE": "dev", "CHRYSALIS_DEV_MODE": "true", "CHRYSALIS_BACKUP_KEEP_WEEKLY": "-1"}, "BACKUP_KEEP_WEEKLY"},
		{"bad ttl", map[string]string{"CHRYSALIS_BASE_URL": "http://localhost:8080", "CHRYSALIS_AUTH_MODE": "dev", "CHRYSALIS_DEV_MODE": "true", "CHRYSALIS_SESSION_TTL": "forever"}, "SESSION_TTL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(envFrom(tt.env))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadHeaderMode(t *testing.T) {
	c, err := Load(envFrom(map[string]string{
		"CHRYSALIS_BASE_URL":            "http://board.example.home",
		"CHRYSALIS_AUTH_MODE":           "header",
		"CHRYSALIS_TRUSTED_PROXY_CIDRS": "10.42.0.0/16, 192.0.2.7,fd00::/8",
		"CHRYSALIS_HEADER_USER":         "X-Forwarded-User",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := []netip.Prefix{
		netip.MustParsePrefix("10.42.0.0/16"),
		netip.MustParsePrefix("192.0.2.7/32"),
		netip.MustParsePrefix("fd00::/8"),
	}
	if len(c.TrustedProxies) != len(want) {
		t.Fatalf("TrustedProxies = %v", c.TrustedProxies)
	}
	for i := range want {
		if c.TrustedProxies[i] != want[i] {
			t.Errorf("TrustedProxies[%d] = %v, want %v", i, c.TrustedProxies[i], want[i])
		}
	}
	if c.Header.User != "X-Forwarded-User" || c.SecureCookies() {
		t.Errorf("unexpected: %+v", c)
	}
}
