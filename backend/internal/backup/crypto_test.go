package backup

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncryptDecryptRoundTripAcrossChunks(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	plain := make([]byte, chunkSize*2+137)
	_, err = rand.Read(plain)
	require.NoError(t, err)

	var encrypted bytes.Buffer
	require.NoError(t, Encrypt(&encrypted, bytes.NewReader(plain), key))
	require.NotContains(t, encrypted.Bytes(), plain[:64])

	var restored bytes.Buffer
	require.NoError(t, Decrypt(&restored, bytes.NewReader(encrypted.Bytes()), key))
	require.Equal(t, plain, restored.Bytes())
}

func TestDecryptRejectsTamperTruncationWrongKeyAndTrailingData(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	var encrypted bytes.Buffer
	require.NoError(t, Encrypt(&encrypted, bytes.NewBufferString("control-plane sentinel"), key))
	archive := encrypted.Bytes()

	tampered := append([]byte(nil), archive...)
	tampered[len(tampered)-1] ^= 0xff
	require.ErrorIs(t, Decrypt(&bytes.Buffer{}, bytes.NewReader(tampered), key), ErrInvalidArchive)

	require.ErrorIs(t, Decrypt(&bytes.Buffer{}, bytes.NewReader(archive[:len(archive)-1]), key), ErrInvalidArchive)

	wrongKey := bytes.Repeat([]byte{0x24}, 32)
	require.ErrorIs(t, Decrypt(&bytes.Buffer{}, bytes.NewReader(archive), wrongKey), ErrInvalidArchive)

	trailing := append(append([]byte(nil), archive...), 0x01)
	require.ErrorIs(t, Decrypt(&bytes.Buffer{}, bytes.NewReader(trailing), key), ErrInvalidArchive)
}

func TestEmptyArchiveIsAuthenticated(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, 32)
	var encrypted bytes.Buffer
	require.NoError(t, Encrypt(&encrypted, bytes.NewReader(nil), key))
	var restored bytes.Buffer
	require.NoError(t, Decrypt(&restored, bytes.NewReader(encrypted.Bytes()), key))
	require.Empty(t, restored.Bytes())
}

func TestDecodeKeyRequiresBase64EncodedAES256Key(t *testing.T) {
	key := bytes.Repeat([]byte{0x31}, 32)
	decoded, err := DecodeKey(base64.StdEncoding.EncodeToString(key))
	require.NoError(t, err)
	require.Equal(t, key, decoded)

	_, err = DecodeKey("not-base64")
	require.Error(t, err)
	_, err = DecodeKey(base64.StdEncoding.EncodeToString(key[:16]))
	require.Error(t, err)
}
