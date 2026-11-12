// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Package config loads settings from CHRYSALIS_* environment variables.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type AuthMode string

const (
	AuthOIDC   AuthMode = "oidc"
	AuthHeader AuthMode = "header"
	AuthDev    AuthMode = "dev"
)

type Config struct {
	ListenAddr string
	BaseURL    *url.URL
	DataDir    string
	AuthMode   AuthMode
	SessionTTL time.Duration
	LogLevel   string
	// TrashRetention is how long trashed items can be restored before they're
	// permanently deleted.
	TrashRetention time.Duration

	OIDC           OIDC
	Header         Header
	TrustedProxies []netip.Prefix
}

type OIDC struct {
	Issuer        string
	ClientID      string
	ClientSecret  string
	RedirectURL   string
	Scopes        []string
	AllowedGroups []string // empty means any authenticated user
}

// Header holds the names of the headers a forward-auth proxy sets.
type Header struct {
	User, Email, Name, Groups string
}

// DBPath is where the SQLite database lives.
func (c *Config) DBPath() string { return filepath.Join(c.DataDir, "chrysalis.db") }

// SecureCookies is true when the app is served over HTTPS.
func (c *Config) SecureCookies() bool { return c.BaseURL.Scheme == "https" }

// Load reads configuration through getenv (os.Getenv in production; a map
// lookup in tests) and validates it.
func Load(getenv func(string) string) (*Config, error) {
	env := func(key, def string) string {
		if v := strings.TrimSpace(getenv("CHRYSALIS_" + key)); v != "" {
			return v
		}
		return def
	}

	c := &Config{
		ListenAddr: env("LISTEN_ADDR", ":8080"),
		DataDir:    env("DATA_DIR", "/data"),
		AuthMode:   AuthMode(env("AUTH_MODE", string(AuthOIDC))),
		LogLevel:   env("LOG_LEVEL", "info"),
		OIDC: OIDC{
			Issuer:        env("OIDC_ISSUER", ""),
			ClientID:      env("OIDC_CLIENT_ID", ""),
			ClientSecret:  env("OIDC_CLIENT_SECRET", ""),
			RedirectURL:   env("OIDC_REDIRECT_URL", ""),
			Scopes:        strings.Fields(env("OIDC_SCOPES", "openid profile email")),
			AllowedGroups: splitList(env("OIDC_ALLOWED_GROUPS", "")),
		},
		Header: Header{
			User:   env("HEADER_USER", "Remote-User"),
			Email:  env("HEADER_EMAIL", "Remote-Email"),
			Name:   env("HEADER_NAME", "Remote-Name"),
			Groups: env("HEADER_GROUPS", "Remote-Groups"),
		},
	}

	var errs []error

	base := env("BASE_URL", "")
	if base == "" {
		errs = append(errs, errors.New("CHRYSALIS_BASE_URL is required"))
	} else if u, err := url.Parse(strings.TrimSuffix(base, "/")); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, fmt.Errorf("CHRYSALIS_BASE_URL %q must be an absolute http(s) URL", base))
	} else {
		c.BaseURL = u
	}

	ttl, err := time.ParseDuration(env("SESSION_TTL", "720h"))
	if err != nil || ttl <= 0 {
		errs = append(errs, errors.New("CHRYSALIS_SESSION_TTL must be a positive duration like 720h"))
	}
	c.SessionTTL = ttl

	days, err := strconv.Atoi(env("TRASH_RETENTION_DAYS", "30"))
	if err != nil || days < 1 {
		errs = append(errs, errors.New("CHRYSALIS_TRASH_RETENTION_DAYS must be a whole number of days, at least 1"))
	}
	c.TrashRetention = time.Duration(days) * 24 * time.Hour

	for _, s := range splitList(env("TRUSTED_PROXY_CIDRS", "")) {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			// Accept a bare IP as a single-address prefix.
			a, aerr := netip.ParseAddr(s)
			if aerr != nil {
				errs = append(errs, fmt.Errorf("CHRYSALIS_TRUSTED_PROXY_CIDRS: invalid entry %q", s))
				continue
			}
			p = netip.PrefixFrom(a, a.BitLen())
		}
		c.TrustedProxies = append(c.TrustedProxies, p.Masked())
	}

	switch c.AuthMode {
	case AuthOIDC:
		if c.OIDC.Issuer == "" || c.OIDC.ClientID == "" || c.OIDC.ClientSecret == "" {
			errs = append(errs, errors.New("oidc mode requires CHRYSALIS_OIDC_ISSUER, CHRYSALIS_OIDC_CLIENT_ID and CHRYSALIS_OIDC_CLIENT_SECRET"))
		}
		if c.OIDC.RedirectURL == "" && c.BaseURL != nil {
			c.OIDC.RedirectURL = c.BaseURL.String() + "/auth/callback"
		}
	case AuthHeader:
		// Without a trusted-proxy list, anyone who can reach the pod could
		// claim to be any user by setting Remote-User themselves.
		if len(c.TrustedProxies) == 0 {
			errs = append(errs, errors.New("header mode requires CHRYSALIS_TRUSTED_PROXY_CIDRS"))
		}
	case AuthDev:
		devOK, _ := strconv.ParseBool(env("DEV_MODE", "false"))
		if !devOK {
			errs = append(errs, errors.New("dev auth mode also requires CHRYSALIS_DEV_MODE=true"))
		}
	default:
		errs = append(errs, fmt.Errorf("CHRYSALIS_AUTH_MODE %q must be oidc, header or dev", c.AuthMode))
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return c, nil
}

func splitList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
}
