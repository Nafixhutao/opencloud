package provisioner

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSeekableBodyCapsSpoolAtLimit(t *testing.T) {
	// One byte over the cap is rejected before the disk fills.
	_, _, err := seekableBody(io.NopCloser(strings.NewReader("123456")), 5)
	require.ErrorIs(t, err, ErrObjectTooLarge)

	// Exactly at the cap spools and rewinds with the full content.
	rs, cleanup, err := seekableBody(io.NopCloser(strings.NewReader("12345")), 5)
	require.NoError(t, err)
	defer cleanup()
	data, err := io.ReadAll(rs)
	require.NoError(t, err)
	require.Equal(t, "12345", string(data))

	// Seekable readers pass through untouched regardless of the cap.
	sr := strings.NewReader("x")
	out, cleanup2, err := seekableBody(sr, 1)
	require.NoError(t, err)
	defer cleanup2()
	require.Same(t, sr, out)
}
