package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestVerifyCodeChallenge_Valid(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	if !verifyCodeChallenge(verifier, challenge) {
		t.Fatal("expected valid PKCE challenge to pass")
	}
}

func TestVerifyCodeChallenge_Invalid(t *testing.T) {
	if verifyCodeChallenge("correct-verifier", "wrong-challenge") {
		t.Fatal("expected invalid PKCE challenge to fail")
	}
}

func TestVerifyCodeChallenge_Empty(t *testing.T) {
	if verifyCodeChallenge("", "challenge") {
		t.Fatal("expected empty verifier to fail")
	}
	if verifyCodeChallenge("verifier", "") {
		t.Fatal("expected empty challenge to fail")
	}
	if verifyCodeChallenge("", "") {
		t.Fatal("expected both empty to fail")
	}
}
