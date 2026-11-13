// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Package webpush sends Web Push messages (docs/DESIGN.md §3.9): a payload
// encrypted for one browser (RFC 8291, aes128gcm), posted to that browser's
// push service with a signed claim of who's sending (VAPID, RFC 8292).
//
// The push service only relays the message; it can't read it. It sees when a
// message is sent, its size, and which subscription it's for.
//
// Everything here is the Go standard library: ECDH and ECDSA on P-256, HKDF,
// and AES-128-GCM.
package webpush

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"syscall"
	"time"
)

var b64 = base64.RawURLEncoding

// decode accepts base64url with or without padding (browsers send either).
func decode(s string) ([]byte, error) {
	for len(s)%4 != 0 && len(s) > 0 && s[len(s)-1] == '=' {
		s = s[:len(s)-1]
	}
	return b64.DecodeString(s)
}

// ---------- VAPID keys (the server's identity) ----------

// Keys is the server's VAPID key pair. Public goes to browsers when they
// subscribe; Private signs every request.
type Keys struct {
	Private *ecdsa.PrivateKey
}

// GenerateKeys makes a new key pair.
func GenerateKeys() (Keys, error) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	return Keys{Private: k}, err
}

// ParseKeys reads a private key saved with Keys.PrivateString.
func ParseKeys(private string) (Keys, error) {
	raw, err := decode(private)
	if err != nil {
		return Keys{}, fmt.Errorf("VAPID private key: %w", err)
	}
	k, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), raw)
	if err != nil {
		return Keys{}, fmt.Errorf("VAPID private key: %w", err)
	}
	return Keys{Private: k}, nil
}

// PrivateString is the private key for storing (base64url of the scalar).
func (k Keys) PrivateString() string {
	b, _ := k.Private.Bytes()
	return b64.EncodeToString(b)
}

// PublicString is the public key as browsers want it: base64url of the
// uncompressed point (the applicationServerKey).
func (k Keys) PublicString() string {
	b, _ := k.Private.PublicKey.Bytes()
	return b64.EncodeToString(b)
}

// ---------- Subscriptions and encryption (RFC 8291) ----------

// Subscription is what a browser's PushManager.subscribe() returns.
type Subscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// Check validates a subscription before it's stored: an https endpoint and
// keys of the right sizes.
func (s Subscription) Check() error {
	u, err := url.Parse(s.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || len(s.Endpoint) > 2048 || u.User != nil {
		return errors.New("the push endpoint must be an https address")
	}
	pub, err := decode(s.Keys.P256dh)
	if err != nil || len(pub) != 65 {
		return errors.New("the subscription's p256dh key isn't valid")
	}
	if _, err := ecdh.P256().NewPublicKey(pub); err != nil {
		return errors.New("the subscription's p256dh key isn't valid")
	}
	if auth, err := decode(s.Keys.Auth); err != nil || len(auth) != 16 {
		return errors.New("the subscription's auth secret isn't valid")
	}
	return nil
}

const recordSize = 4096

// MaxPayload is the largest plaintext that fits one record.
const MaxPayload = recordSize - 16 - 1 - 86 // tag, delimiter, header

// Encrypt encrypts plaintext for a subscription (aes128gcm, one record).
func Encrypt(plaintext []byte, sub Subscription) ([]byte, error) {
	asKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	return encrypt(plaintext, sub, asKey, salt)
}

func encrypt(plaintext []byte, sub Subscription, asKey *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	if len(plaintext) > MaxPayload {
		return nil, fmt.Errorf("payload is %d bytes; at most %d fit", len(plaintext), MaxPayload)
	}
	uaPub, err := decode(sub.Keys.P256dh)
	if err != nil {
		return nil, err
	}
	auth, err := decode(sub.Keys.Auth)
	if err != nil {
		return nil, err
	}
	uaKey, err := ecdh.P256().NewPublicKey(uaPub)
	if err != nil {
		return nil, err
	}
	shared, err := asKey.ECDH(uaKey)
	if err != nil {
		return nil, err
	}
	asPub := asKey.PublicKey().Bytes()

	// IKM from the shared secret, keyed by the browser's auth secret and both
	// public keys (RFC 8291 §3.3), then the content key and nonce from the
	// salt (RFC 8188 §2.2, §2.3).
	prkKey, err := hkdf.Extract(sha256.New, shared, auth)
	if err != nil {
		return nil, err
	}
	keyInfo := "WebPush: info\x00" + string(uaPub) + string(asPub)
	ikm, err := hkdf.Expand(sha256.New, prkKey, keyInfo, 32)
	if err != nil {
		return nil, err
	}
	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	record := append(append([]byte{}, plaintext...), 0x02) // last (only) record, no padding

	var out bytes.Buffer
	out.Write(salt)
	_ = binary.Write(&out, binary.BigEndian, uint32(recordSize))
	if len(asPub) != 65 {
		return nil, errors.New("unexpected public key length")
	}
	out.WriteByte(65) // an uncompressed P-256 point
	out.Write(asPub)
	out.Write(gcm.Seal(nil, nonce, record, nil))
	return out.Bytes(), nil
}

// ---------- VAPID (RFC 8292) ----------

// vapidHeader signs a short-lived claim for the push service at endpoint.
func vapidHeader(keys Keys, endpoint, subject string, now time.Time) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	claims, _ := json.Marshal(map[string]any{
		"aud": u.Scheme + "://" + u.Host,
		"exp": now.Add(12 * time.Hour).Unix(),
		"sub": subject,
	})
	signing := b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`)) + "." + b64.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, keys.Private, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return "vapid t=" + signing + "." + b64.EncodeToString(sig) + ", k=" + keys.PublicString(), nil
}

// ---------- Sending ----------

// ErrGone means the subscription no longer exists (the browser unsubscribed
// or the app was removed); forget it.
var ErrGone = errors.New("push subscription is gone")

// Sender posts messages to push services.
type Sender struct {
	Keys    Keys
	Subject string // a mailto: or https: address for the push service to contact
	Client  *http.Client
}

// NewSender makes a sender whose HTTP client only connects to public
// addresses: endpoints come from browsers, so a made-up one must not be able
// to point the server at something on its own network.
func NewSender(keys Keys, subject string) *Sender {
	d := &net.Dialer{Timeout: 10 * time.Second, Control: publicOnly}
	tr := &http.Transport{
		DialContext:           d.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &Sender{Keys: keys, Subject: subject, Client: &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
		// Push services answer directly; a redirect is not followed.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func publicOnly(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return fmt.Errorf("push endpoint address %s is not public", host)
	}
	return nil
}

// Send encrypts payload for sub and posts it. ttl is how long the push
// service may hold it for an offline device.
func (s *Sender) Send(ctx context.Context, sub Subscription, payload []byte, ttl time.Duration) error {
	body, err := Encrypt(payload, sub)
	if err != nil {
		return err
	}
	auth, err := vapidHeader(s.Keys, sub.Endpoint, s.Subject, time.Now())
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", strconv.Itoa(int(ttl.Seconds())))
	req.Header.Set("Urgency", "normal")
	req.Header.Set("Authorization", auth)
	res, err := s.Client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	switch {
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone:
		return ErrGone
	case res.StatusCode >= 300:
		return fmt.Errorf("push service answered %s", res.Status)
	}
	return nil
}
