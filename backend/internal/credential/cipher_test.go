package credential

import (
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func testCipher(t *testing.T) *Cipher {
	t.Helper()
	key := make([]byte, keySize)
	_, err := rand.Read(key)
	require.NoError(t, err)
	c, err := New(base64.StdEncoding.EncodeToString(key))
	require.NoError(t, err)
	return c
}

func TestCipherRoundTripBindsDatabaseID(t *testing.T) {
	c := testCipher(t)
	databaseID := uuid.New()
	plaintext := []byte(`{"password":"never log this"}`)

	sealed, err := c.Encrypt(databaseID, plaintext)
	require.NoError(t, err)
	require.NotContains(t, string(sealed), "never log this")

	opened, err := c.Decrypt(databaseID, sealed)
	require.NoError(t, err)
	require.Equal(t, plaintext, opened)

	_, err = c.Decrypt(uuid.New(), sealed)
	require.ErrorContains(t, err, "authentication failed")
}

func TestCipherRejectsTamperTruncationAndBadKeys(t *testing.T) {
	c := testCipher(t)
	databaseID := uuid.New()
	sealed, err := c.Encrypt(databaseID, []byte("secret"))
	require.NoError(t, err)

	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 0xff
	_, err = c.Decrypt(databaseID, tampered)
	require.ErrorContains(t, err, "authentication failed")

	_, err = c.Decrypt(databaseID, sealed[:len(sealed)-1])
	require.Error(t, err)

	_, err = New("not-base64")
	require.Error(t, err)
	_, err = New(base64.StdEncoding.EncodeToString(make([]byte, 16)))
	require.Error(t, err)
}
