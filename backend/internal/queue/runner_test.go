package queue

import (
	"testing"
	"time"
)

func TestRetryBackoffIsExponentialAndCapped(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: time.Second},
		{attempt: 1, want: time.Second},
		{attempt: 2, want: 2 * time.Second},
		{attempt: 5, want: 16 * time.Second},
		{attempt: 20, want: 256 * time.Second},
	}
	for _, tt := range tests {
		if got := retryBackoff(tt.attempt); got != tt.want {
			t.Fatalf("retryBackoff(%d) = %s, want %s", tt.attempt, got, tt.want)
		}
	}
}
