package oauth

import (
	"testing"
	"time"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := deriveKey("test-secret-key-for-testing")

	original := map[string]string{"hello": "world", "foo": "bar"}
	encoded, err := encrypt(key, original)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	var decoded map[string]string
	if err := decrypt(key, encoded, &decoded); err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if decoded["hello"] != "world" || decoded["foo"] != "bar" {
		t.Fatalf("unexpected decoded value: %v", decoded)
	}
}

func TestDecryptTamperedData(t *testing.T) {
	key := deriveKey("test-secret")

	encoded, err := encrypt(key, map[string]string{"a": "b"})
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Tamper with the ciphertext
	tampered := []byte(encoded)
	if len(tampered) > 5 {
		tampered[5] ^= 0xff
	}

	var decoded map[string]string
	if err := decrypt(key, string(tampered), &decoded); err == nil {
		t.Fatal("expected error for tampered data, got nil")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := deriveKey("key-one")
	key2 := deriveKey("key-two")

	encoded, err := encrypt(key1, "secret data")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	var decoded string
	if err := decrypt(key2, encoded, &decoded); err == nil {
		t.Fatal("expected error for wrong key, got nil")
	}
}

func TestStatePayloadTTL(t *testing.T) {
	key := deriveKey("test-ttl")

	s := StatePayload{
		RedirectURI:   "http://localhost:1234/callback",
		CodeChallenge: "abc123",
		ClientID:      "test-client",
		ClientState:   "opaque-state",
	}

	encoded, err := encryptState(key, s)
	if err != nil {
		t.Fatalf("encryptState: %v", err)
	}

	// Should succeed within TTL
	decoded, err := decryptState(key, encoded, 5*time.Minute)
	if err != nil {
		t.Fatalf("decryptState: %v", err)
	}
	if decoded.RedirectURI != s.RedirectURI {
		t.Fatalf("unexpected redirect_uri: %s", decoded.RedirectURI)
	}

	// Should fail with very short TTL
	_, err = decryptState(key, encoded, 0)
	if err == nil {
		t.Fatal("expected TTL error, got nil")
	}
}

func TestAuthCodePayloadTTL(t *testing.T) {
	key := deriveKey("test-authcode")

	a := AuthCodePayload{
		AccessToken:   "access-token-123",
		IDToken:       "id-token-456",
		RefreshToken:  "refresh-token-789",
		TokenType:     "Bearer",
		ExpiresIn:     3600,
		CodeChallenge: "challenge",
		ClientID:      "client",
		RedirectURI:   "http://localhost:5000/cb",
	}

	encoded, err := encryptAuthCode(key, a)
	if err != nil {
		t.Fatalf("encryptAuthCode: %v", err)
	}

	// Should succeed within TTL
	decoded, err := decryptAuthCode(key, encoded, 60*time.Second)
	if err != nil {
		t.Fatalf("decryptAuthCode: %v", err)
	}
	if decoded.AccessToken != a.AccessToken {
		t.Fatalf("unexpected access_token: %s", decoded.AccessToken)
	}
	if decoded.RefreshToken != a.RefreshToken {
		t.Fatalf("unexpected refresh_token: %s", decoded.RefreshToken)
	}

	// Should fail with very short TTL
	_, err = decryptAuthCode(key, encoded, 0)
	if err == nil {
		t.Fatal("expected TTL error, got nil")
	}
}
