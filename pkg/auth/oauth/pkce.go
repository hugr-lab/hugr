package oauth

import (
	"crypto/sha256"
	"encoding/base64"
)

// verifyCodeChallenge validates a PKCE S256 code_verifier against a code_challenge.
// Returns true if SHA256(code_verifier) == code_challenge (base64url-encoded, no padding).
func verifyCodeChallenge(verifier, challenge string) bool {
	if verifier == "" || challenge == "" {
		return false
	}
	h := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	return computed == challenge
}
