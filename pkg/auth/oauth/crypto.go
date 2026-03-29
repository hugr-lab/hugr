package oauth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// StatePayload is encrypted into the OAuth state parameter.
// Carried from /oauth/authorize → OIDC provider → /oauth/callback.
type StatePayload struct {
	RedirectURI   string `json:"redirect_uri"`
	CodeChallenge string `json:"code_challenge"`
	ClientID      string `json:"client_id"`
	ClientState   string `json:"client_state"`
	Timestamp     int64  `json:"timestamp"`
}

// AuthCodePayload is encrypted into the authorization code.
// Carried from /oauth/callback → MCP client → /oauth/token.
type AuthCodePayload struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	CodeChallenge string `json:"code_challenge"`
	ClientID     string `json:"client_id"`
	RedirectURI  string `json:"redirect_uri"`
	Timestamp    int64  `json:"timestamp"`
}

// deriveKey derives a 32-byte AES-256 key from an arbitrary secret string.
func deriveKey(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// encrypt serializes v to JSON, encrypts with AES-256-GCM, and returns base64url.
func encrypt(key []byte, v any) (string, error) {
	plaintext, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("rand nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// decrypt decodes base64url, decrypts AES-256-GCM, and unmarshals JSON into v.
func decrypt(key []byte, encoded string, v any) error {
	ciphertext, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("base64 decode: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("new gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}

	return json.Unmarshal(plaintext, v)
}

// encryptState encrypts a StatePayload, setting the timestamp to now.
func encryptState(key []byte, s StatePayload) (string, error) {
	s.Timestamp = time.Now().Unix()
	return encrypt(key, s)
}

// decryptState decrypts and validates a StatePayload with TTL check.
func decryptState(key []byte, encoded string, ttl time.Duration) (*StatePayload, error) {
	var s StatePayload
	if err := decrypt(key, encoded, &s); err != nil {
		return nil, err
	}
	if time.Since(time.Unix(s.Timestamp, 0)) > ttl {
		return nil, fmt.Errorf("state expired")
	}
	return &s, nil
}

// encryptAuthCode encrypts an AuthCodePayload, setting the timestamp to now.
func encryptAuthCode(key []byte, a AuthCodePayload) (string, error) {
	a.Timestamp = time.Now().Unix()
	return encrypt(key, a)
}

// decryptAuthCode decrypts and validates an AuthCodePayload with TTL check.
func decryptAuthCode(key []byte, encoded string, ttl time.Duration) (*AuthCodePayload, error) {
	var a AuthCodePayload
	if err := decrypt(key, encoded, &a); err != nil {
		return nil, err
	}
	if time.Since(time.Unix(a.Timestamp, 0)) > ttl {
		return nil, fmt.Errorf("authorization code expired")
	}
	return &a, nil
}
