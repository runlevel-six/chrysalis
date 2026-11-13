// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package webpush

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The worked example in RFC 8291, Appendix A.
const (
	rfcPlaintext = "When I grow up, I want to be a watermelon"
	rfcASPrivate = "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"
	rfcUAPrivate = "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"
	rfcUAPublic  = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	rfcSalt      = "DGv6ra1nlYgDCS1FRnbzlw"
	rfcAuth      = "BTBZMqHH6r4Tts7J_aSIgg" // betterleaks:allow (RFC 8291 Appendix A example value, published)
	rfcMessage   = "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
)

func rfcSub() Subscription {
	var s Subscription
	s.Endpoint = "https://push.example.net/push/JzLQ3raZJfFBR0aqvOMsLrt54w4rJUsV"
	s.Keys.P256dh = rfcUAPublic
	s.Keys.Auth = rfcAuth
	return s
}

func TestRFC8291Example(t *testing.T) {
	asRaw, _ := decode(rfcASPrivate)
	as, err := ecdh.P256().NewPrivateKey(asRaw)
	if err != nil {
		t.Fatal(err)
	}
	salt, _ := decode(rfcSalt)
	got, err := encrypt([]byte(rfcPlaintext), rfcSub(), as, salt)
	if err != nil {
		t.Fatal(err)
	}
	if b64.EncodeToString(got) != rfcMessage {
		t.Fatalf("message\n got %s\nwant %s", b64.EncodeToString(got), rfcMessage)
	}
}

// decrypt is the browser's side, to check round trips.
func decrypt(t *testing.T, msg []byte, uaPriv *ecdh.PrivateKey, auth []byte) []byte {
	t.Helper()
	salt, idlen := msg[:16], int(msg[20])
	asPub := msg[21 : 21+idlen]
	ct := msg[21+idlen:]
	as, err := ecdh.P256().NewPublicKey(asPub)
	if err != nil {
		t.Fatal(err)
	}
	shared, _ := uaPriv.ECDH(as)
	prkKey, _ := hkdf.Extract(sha256.New, shared, auth)
	ikm, _ := hkdf.Expand(sha256.New, prkKey, "WebPush: info\x00"+string(uaPriv.PublicKey().Bytes())+string(asPub), 32)
	prk, _ := hkdf.Extract(sha256.New, ikm, salt)
	cek, _ := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pt[len(pt)-1] != 0x02 {
		t.Fatalf("delimiter = %x", pt[len(pt)-1])
	}
	return pt[:len(pt)-1]
}

func TestEncryptRoundTrip(t *testing.T) {
	uaRaw, _ := decode(rfcUAPrivate)
	ua, _ := ecdh.P256().NewPrivateKey(uaRaw)
	auth, _ := decode(rfcAuth)
	msg, err := Encrypt([]byte(`{"title":"Chrysalis","body":"Heads up"}`), rfcSub())
	if err != nil {
		t.Fatal(err)
	}
	if got := decrypt(t, msg, ua, auth); string(got) != `{"title":"Chrysalis","body":"Heads up"}` {
		t.Fatalf("round trip = %q", got)
	}
	if _, err := Encrypt(make([]byte, MaxPayload+1), rfcSub()); err == nil {
		t.Error("oversized payload accepted")
	}
}

func TestKeysRoundTrip(t *testing.T) {
	k, err := GenerateKeys()
	if err != nil {
		t.Fatal(err)
	}
	k2, err := ParseKeys(k.PrivateString())
	if err != nil {
		t.Fatal(err)
	}
	if k2.PublicString() != k.PublicString() || len(k.PublicString()) != 87 {
		t.Fatalf("public %q vs %q", k2.PublicString(), k.PublicString())
	}
	if _, err := ParseKeys("not a key"); err == nil {
		t.Error("garbage accepted")
	}
}

func TestSubscriptionCheck(t *testing.T) {
	if err := rfcSub().Check(); err != nil {
		t.Fatal(err)
	}
	bad := []func(*Subscription){
		func(s *Subscription) { s.Endpoint = "http://push.example.net/x" },
		func(s *Subscription) { s.Endpoint = "https://user:pw@push.example.net/x" },
		func(s *Subscription) { s.Keys.P256dh = "AAAA" },
		func(s *Subscription) { s.Keys.Auth = "short" },
	}
	for i, f := range bad {
		s := rfcSub()
		f(&s)
		if s.Check() == nil {
			t.Errorf("case %d accepted", i)
		}
	}
}

func TestSendSignsAndPosts(t *testing.T) {
	keys, _ := GenerateKeys()
	var gotAuth, gotEnc, gotTTL string
	var bodyLen int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotEnc, gotTTL = r.Header.Get("Authorization"), r.Header.Get("Content-Encoding"), r.Header.Get("TTL")
		b := make([]byte, 8192)
		n, _ := r.Body.Read(b)
		bodyLen = n
		if strings.HasSuffix(r.URL.Path, "/gone") {
			w.WriteHeader(http.StatusGone)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	s := NewSender(keys, "mailto:admin@example.com")
	s.Client = srv.Client() // the test server is on loopback, which NewSender refuses
	sub := rfcSub()
	sub.Endpoint = srv.URL + "/push/abc"
	if err := s.Send(context.Background(), sub, []byte("hello"), time.Hour); err != nil {
		t.Fatal(err)
	}
	if gotEnc != "aes128gcm" || gotTTL != "3600" || bodyLen < 86+5+17 {
		t.Errorf("headers enc=%q ttl=%q body=%d", gotEnc, gotTTL, bodyLen)
	}
	// The VAPID token verifies with the public key, for the endpoint's origin.
	tok := strings.TrimPrefix(strings.Split(gotAuth, ", k=")[0], "vapid t=")
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("token %q", tok)
	}
	var claims struct {
		Aud string
		Sub string
		Exp int64
	}
	raw, _ := b64.DecodeString(parts[1])
	_ = json.Unmarshal(raw, &claims)
	if claims.Aud != srv.URL || claims.Sub != "mailto:admin@example.com" || claims.Exp < time.Now().Unix() {
		t.Errorf("claims %+v", claims)
	}
	sig, _ := b64.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, ss := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	pub, _ := decode(keys.PublicString())
	pk, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), pub)
	if err != nil {
		t.Fatal(err)
	}
	if !ecdsa.Verify(pk, sum[:], r, ss) {
		t.Error("VAPID signature doesn't verify")
	}

	sub.Endpoint = srv.URL + "/gone"
	if err := s.Send(context.Background(), sub, []byte("x"), time.Hour); !errors.Is(err, ErrGone) {
		t.Errorf("gone: %v", err)
	}
}

func TestSenderRefusesPrivateAddresses(t *testing.T) {
	keys, _ := GenerateKeys()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(201) }))
	defer srv.Close()
	s := NewSender(keys, "mailto:admin@example.com")
	sub := rfcSub()
	sub.Endpoint = srv.URL + "/push"
	err := s.Send(context.Background(), sub, []byte("x"), time.Hour)
	if err == nil || !strings.Contains(err.Error(), "not public") {
		t.Fatalf("loopback endpoint: %v", err)
	}
}
