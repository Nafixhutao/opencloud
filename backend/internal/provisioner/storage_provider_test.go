package provisioner_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/provisioner"
)

func TestStorageProviderSelection(t *testing.T) {
	t.Parallel()

	t.Run("fake-provider-constructs", func(t *testing.T) {
		provider := provisioner.NewFakeStorageProvider()
		require.NotNil(t, provider)
		require.Implements(t, (*provisioner.ObjectStorageProvider)(nil), provider)
	})

	t.Run("fake-provider-create-delete-lifecycle", func(t *testing.T) {
		ctx := context.Background()
		provider := provisioner.NewFakeStorageProvider()

		physicalName := "test-bucket-lifecycle"

		err := provider.CreateBucket(ctx, provisioner.BucketSpec{
			PhysicalName: physicalName,
			Visibility:   "private",
		})
		require.NoError(t, err)

		exists, err := provider.BucketExists(ctx, provisioner.BucketRef{
			PhysicalName: physicalName,
		})
		require.NoError(t, err)
		require.True(t, exists)

		err = provider.DeleteBucket(ctx, provisioner.BucketRef{
			PhysicalName: physicalName,
		})
		require.NoError(t, err)

		exists, err = provider.BucketExists(ctx, provisioner.BucketRef{
			PhysicalName: physicalName,
		})
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("fake-provider-rejects-duplicate-create", func(t *testing.T) {
		ctx := context.Background()
		provider := provisioner.NewFakeStorageProvider()

		err := provider.CreateBucket(ctx, provisioner.BucketSpec{
			PhysicalName: "duplicate-bucket",
			Visibility:   "private",
		})
		require.NoError(t, err)

		err = provider.CreateBucket(ctx, provisioner.BucketSpec{
			PhysicalName: "duplicate-bucket",
			Visibility:   "private",
		})
		require.ErrorIs(t, err, provisioner.ErrBucketExists)
	})

	t.Run("fake-provider-blocks-non-empty-delete", func(t *testing.T) {
		ctx := context.Background()
		provider := provisioner.NewFakeStorageProvider()

		physicalName := "nonempty-bucket"
		provider.SetBucketsForTest(map[string]*provisioner.FakeBucketState{
			physicalName: {
				PhysicalName: physicalName,
				Objects: map[string]*provisioner.FakeObjectState{
					"obj1": {Key: "obj1", Data: []byte("x")},
				},
			},
		})

		err := provider.DeleteBucket(ctx, provisioner.BucketRef{
			PhysicalName: physicalName,
		})
		require.Error(t, err)
		require.True(t, provisioner.IsBucketNotEmpty(err))
	})

	t.Run("fake-provider-reports-missing-bucket", func(t *testing.T) {
		ctx := context.Background()
		provider := provisioner.NewFakeStorageProvider()

		err := provider.DeleteBucket(ctx, provisioner.BucketRef{
			PhysicalName: "nonexistent",
		})
		require.ErrorIs(t, err, provisioner.ErrBucketNotFound)
	})

	t.Run("s3-provider-validation-fails-on-missing-config", func(t *testing.T) {
		ctx := context.Background()

		tests := []struct {
			name    string
			cfg     provisioner.S3StorageConfig
			wantErr string
		}{
			{
				name:    "missing endpoint",
				cfg:     provisioner.S3StorageConfig{Region: "us-east-1", AccessKeyID: "key", SecretAccessKey: "secret"},
				wantErr: "endpoint",
			},
			{
				name:    "missing region",
				cfg:     provisioner.S3StorageConfig{Endpoint: "http://rustfs:9000", AccessKeyID: "key", SecretAccessKey: "secret"},
				wantErr: "region",
			},
			{
				name:    "missing access key ID",
				cfg:     provisioner.S3StorageConfig{Endpoint: "http://rustfs:9000", Region: "us-east-1", SecretAccessKey: "secret"},
				wantErr: "access key",
			},
			{
				name:    "missing secret access key",
				cfg:     provisioner.S3StorageConfig{Endpoint: "http://rustfs:9000", Region: "us-east-1", AccessKeyID: "key"},
				wantErr: "secret",
			},
		}

		for _, tt := range tests {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				provider, err := provisioner.NewS3StorageProvider(ctx, tt.cfg)
				require.Error(t, err)
				require.Nil(t, provider)
				require.Contains(t, strings.ToLower(err.Error()), tt.wantErr)
			})
		}
	})

	t.Run("s3-provider-fails-on-unreachable-endpoint", func(t *testing.T) {
		ctx := context.Background()
		cfg := provisioner.S3StorageConfig{
			Endpoint:        "http://127.0.0.1:1",
			Region:          "us-east-1",
			AccessKeyID:     "key",
			SecretAccessKey: "secret",
			UsePathStyle:    true,
		}
		provider, err := provisioner.NewS3StorageProvider(ctx, cfg)
		require.NoError(t, err)
		require.NotNil(t, provider)

		_, err = provider.BucketExists(ctx, provisioner.BucketRef{
			PhysicalName: "test-bucket",
		})
		require.Error(t, err)
	})

	t.Run("s3-provider-available-when-configured", func(t *testing.T) {
		endpoint := os.Getenv("STORAGE_S3_ENDPOINT")
		if endpoint == "" {
			t.Skip("STORAGE_S3_ENDPOINT not set; skipping S3 connectivity test")
		}

		cfg := provisioner.S3StorageConfig{
			Endpoint:        endpoint,
			Region:          "us-east-1",
			AccessKeyID:     os.Getenv("STORAGE_S3_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("STORAGE_S3_SECRET_ACCESS_KEY"),
			UsePathStyle:    true,
		}
		if cfg.AccessKeyID == "" {
			cfg.AccessKeyID = "rustfsadmin"
		}
		if cfg.SecretAccessKey == "" {
			cfg.SecretAccessKey = "rustfsadmin"
		}

		ctx := context.Background()
		provider, err := provisioner.NewS3StorageProvider(ctx, cfg)
		require.NoError(t, err)
		require.NotNil(t, provider)

		exists, err := provider.BucketExists(ctx, provisioner.BucketRef{
			PhysicalName: "nonexistent-check-bucket",
		})
		require.NoError(t, err)
		require.False(t, exists)
	})
}
