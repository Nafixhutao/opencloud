package domainverify

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSignerReconstructsWithoutPersistingPlaintext(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	signer, err := New(key)
	require.NoError(t, err)

	domainID := uuid.New()
	accountID := uuid.New()
	expiresAt, digest, err := signer.Issue(
		time.Date(2026, 7, 30, 1, 2, 3, 456789000, time.UTC),
		time.Hour,
		domainID,
		accountID,
		"www.example.com",
	)
	require.NoError(t, err)
	token := signer.Token(domainID, accountID, "www.example.com", expiresAt)
	require.True(t, Matches(token, digest))
	require.False(t, Matches(token+"tampered", digest))
	require.NotContains(t, string(digest), token)
}

func TestNewRejectsMalformedKeys(t *testing.T) {
	_, err := New("not-base64")
	require.Error(t, err)
	_, err = New(base64.StdEncoding.EncodeToString([]byte("short")))
	require.Error(t, err)
}
