package middleware

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidRequestID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "uuid", id: "0190d8e5-8f7a-7c31-a5de-9b113817a31d", want: true},
		{name: "trace id", id: "edge:trace_123.abc", want: true},
		{name: "empty", id: "", want: false},
		{name: "whitespace", id: "not trusted", want: false},
		{name: "oversized", id: strings.Repeat("a", maxRequestIDLength+1), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, validRequestID(tt.id))
		})
	}
}
