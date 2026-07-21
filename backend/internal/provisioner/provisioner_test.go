package provisioner

import (
	"testing"

	"github.com/google/uuid"
)

func TestParseBackend(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    Backend
		wantErr bool
	}{
		{name: "empty defaults to Docker", want: BackendDocker},
		{name: "Docker is normalized", value: " Docker ", want: BackendDocker},
		{name: "Hestia fallback", value: "hestia", want: BackendHestia},
		{name: "fake test backend", value: "fake", want: BackendFake},
		{name: "unknown backend", value: "kubernetes", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBackend(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ParseBackend() succeeded for an unsupported backend")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseBackend() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseBackend() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResourceNameIsDeterministic(t *testing.T) {
	id := uuid.MustParse("2b8b66f1-1a25-4f7f-8bb3-c4c54feaf4a1")
	const want = "opencloud-site-2b8b66f1-1a25-4f7f-8bb3-c4c54feaf4a1"

	if got := ResourceName(id); got != want {
		t.Fatalf("ResourceName() = %q, want %q", got, want)
	}
}
