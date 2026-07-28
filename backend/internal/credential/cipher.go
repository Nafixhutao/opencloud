// Package credential encrypts one-time customer database credentials before
// they enter the control-plane database.
package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

const (
	envelopeVersion byte = 1
	keySize         int  = 32
)

var envelopeMagic = [4]byte{'O', 'C', 'D', 'C'}

// Cipher seals small credential payloads with AES-256-GCM. The managed
// database UUID is authenticated as associated data, so ciphertext cannot be
// moved to another resource row.
type Cipher struct {
	aead cipher.AEAD
}

// New parses a base64-encoded 32-byte key.
func New(encodedKey string) (*Cipher, error) {
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		return nil, errors.New("credential encryption key must be valid base64")
	}
	if len(key) != keySize {
		return nil, fmt.Errorf("credential encryption key must decode to %d bytes", keySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initialize credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize credential GCM: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns a versioned nonce+ciphertext envelope.
func (c *Cipher) Encrypt(databaseID uuid.UUID, plaintext []byte) ([]byte, error) {
	if c == nil || c.aead == nil {
		return nil, errors.New("credential cipher is not configured")
	}
	if databaseID == uuid.Nil {
		return nil, errors.New("database id is required")
	}

	headerLen := len(envelopeMagic) + 1
	out := make([]byte, headerLen+c.aead.NonceSize())
	copy(out, envelopeMagic[:])
	out[len(envelopeMagic)] = envelopeVersion
	nonce := out[headerLen:]
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate credential nonce: %w", err)
	}
	aad := associatedData(databaseID)
	out = c.aead.Seal(out, nonce, plaintext, aad)
	return out, nil
}

// Decrypt authenticates and opens one credential envelope.
func (c *Cipher) Decrypt(databaseID uuid.UUID, envelope []byte) ([]byte, error) {
	if c == nil || c.aead == nil {
		return nil, errors.New("credential cipher is not configured")
	}
	headerLen := len(envelopeMagic) + 1
	minLen := headerLen + c.aead.NonceSize() + c.aead.Overhead()
	if len(envelope) < minLen {
		return nil, errors.New("credential envelope is truncated")
	}
	if string(envelope[:len(envelopeMagic)]) != string(envelopeMagic[:]) {
		return nil, errors.New("credential envelope magic is invalid")
	}
	if envelope[len(envelopeMagic)] != envelopeVersion {
		return nil, errors.New("credential envelope version is unsupported")
	}
	nonce := envelope[headerLen : headerLen+c.aead.NonceSize()]
	ciphertext := envelope[headerLen+c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, associatedData(databaseID))
	if err != nil {
		return nil, errors.New("credential envelope authentication failed")
	}
	return plaintext, nil
}

func associatedData(databaseID uuid.UUID) []byte {
	aad := make([]byte, 0, len(envelopeMagic)+1+len(databaseID))
	aad = append(aad, envelopeMagic[:]...)
	aad = append(aad, envelopeVersion)
	aad = append(aad, databaseID[:]...)
	return aad
}
