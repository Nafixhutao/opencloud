package provisioner_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/provisioner"
)

func TestS3StorageConfigValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     provisioner.S3StorageConfig
		wantErr bool
	}{
		{
			name: "all fields set succeeds",
			cfg: provisioner.S3StorageConfig{
				Endpoint: "http://rustfs:9000", Region: "us-east-1",
				AccessKeyID: "rustfsadmin", SecretAccessKey: "rustfsadmin", UsePathStyle: true,
			},
			wantErr: false,
		},
		{
			name: "empty endpoint fails",
			cfg: provisioner.S3StorageConfig{
				Endpoint: "", Region: "us-east-1", AccessKeyID: "key", SecretAccessKey: "secret",
			},
			wantErr: true,
		},
		{
			name: "empty region fails",
			cfg: provisioner.S3StorageConfig{
				Endpoint: "http://rustfs:9000", Region: "", AccessKeyID: "key", SecretAccessKey: "secret",
			},
			wantErr: true,
		},
		{
			name: "empty access key ID fails",
			cfg: provisioner.S3StorageConfig{
				Endpoint: "http://rustfs:9000", Region: "us-east-1", AccessKeyID: "", SecretAccessKey: "secret",
			},
			wantErr: true,
		},
		{
			name: "empty secret access key fails",
			cfg: provisioner.S3StorageConfig{
				Endpoint: "http://rustfs:9000", Region: "us-east-1", AccessKeyID: "key", SecretAccessKey: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			provider, err := provisioner.NewS3StorageProvider(ctx, tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, provider)
			} else {
				require.NoError(t, err)
				require.NotNil(t, provider)
			}
		})
	}
}

func TestS3StorageIntegrationEndToEnd(t *testing.T) {
	endpoint := os.Getenv("STORAGE_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("STORAGE_S3_ENDPOINT not set; skipping S3 integration test")
	}

	cfg := provisioner.S3StorageConfig{
		Endpoint:        endpoint,
		Region:          os.Getenv("STORAGE_S3_REGION"),
		AccessKeyID:     os.Getenv("STORAGE_S3_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("STORAGE_S3_SECRET_ACCESS_KEY"),
		UsePathStyle:    true,
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	ctx := context.Background()
	provider, err := provisioner.NewS3StorageProvider(ctx, cfg)
	require.NoError(t, err)

	bucketName := "opencloud-test-" + t.Name()

	t.Run("create-bucket", func(t *testing.T) {
		spec := provisioner.BucketSpec{
			PhysicalName: bucketName,
			Visibility:   "private",
		}
		err := provider.CreateBucket(ctx, spec)
		require.NoError(t, err)
	})

	t.Run("bucket-exists", func(t *testing.T) {
		ref := provisioner.BucketRef{
			PhysicalName: bucketName,
		}
		exists, err := provider.BucketExists(ctx, ref)
		require.NoError(t, err)
		require.True(t, exists)
	})

	t.Run("duplicate-create", func(t *testing.T) {
		spec := provisioner.BucketSpec{
			PhysicalName: bucketName,
		}
		err := provider.CreateBucket(ctx, spec)
		require.ErrorIs(t, err, provisioner.ErrBucketExists)
	})

	t.Run("bucket-not-found", func(t *testing.T) {
		ref := provisioner.BucketRef{
			PhysicalName: "nonexistent-bucket-opencloud-test",
		}
		exists, err := provider.BucketExists(ctx, ref)
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("delete-bucket", func(t *testing.T) {
		ref := provisioner.BucketRef{
			PhysicalName: bucketName,
		}
		err := provider.DeleteBucket(ctx, ref)
		require.NoError(t, err)
	})

	t.Run("delete-missing-bucket", func(t *testing.T) {
		ref := provisioner.BucketRef{
			PhysicalName: bucketName,
		}
		err := provider.DeleteBucket(ctx, ref)
		require.ErrorIs(t, err, provisioner.ErrBucketNotFound)
	})
}
