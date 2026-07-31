// Package domainverify creates reproducible DNS ownership challenges without
// persisting their plaintext value in PostgreSQL.
package domainverify

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const tokenPrefix = "oc_verify_"

// Signer derives a DNS challenge from immutable domain fields and an external
// 256-bit key. Only a digest, expiry, and consumed marker are stored in the DB.
type Signer struct {
	key [32]byte
}

// New parses a base64-encoded 32-byte domain verification key.
func New(encodedKey string) (*Signer, error) {
	raw, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, errors.New("domain verification key must be valid base64")
	}
	if len(raw) != 32 {
		clear(raw)
		return nil, errors.New("domain verification key must decode to 32 bytes")
	}
	var key [32]byte
	copy(key[:], raw)
	clear(raw)
	return &Signer{key: key}, nil
}

// Token reconstructs the challenge value shown to the customer.
func (s *Signer) Token(
	domainID, accountID uuid.UUID,
	hostname string,
	expiresAt time.Time,
) string {
	mac := hmac.New(sha256.New, s.key[:])
	_, _ = mac.Write([]byte("opencloud-domain-verification-v1\x00"))
	_, _ = mac.Write([]byte(domainID.String()))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(accountID.String()))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(hostname))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(strconv.FormatInt(expiresAt.UTC().UnixMicro(), 10)))
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Digest returns the SHA-256 marker persisted for constant-time verification.
func Digest(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// Matches reports whether token corresponds to the stored digest.
func Matches(token string, digest []byte) bool {
	actual := Digest(token)
	return len(digest) == sha256.Size && subtle.ConstantTimeCompare(actual, digest) == 1
}

// Issue returns a microsecond-stable expiry and its corresponding digest.
func (s *Signer) Issue(
	now time.Time,
	ttl time.Duration,
	domainID, accountID uuid.UUID,
	hostname string,
) (time.Time, []byte, error) {
	if ttl <= 0 {
		return time.Time{}, nil, fmt.Errorf("verification ttl must be positive")
	}
	expiresAt := now.UTC().Add(ttl).Truncate(time.Microsecond)
	token := s.Token(domainID, accountID, hostname, expiresAt)
	return expiresAt, Digest(token), nil
}
