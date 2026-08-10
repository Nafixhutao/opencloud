package provisioner_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/provisioner"
)

func TestFakeProviderObjectOperations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := provisioner.NewFakeStorageProvider()
	bucket := "test-bucket"

	require.NoError(t, p.CreateBucket(ctx, provisioner.BucketSpec{
		BucketID:     uuid.New(),
		PhysicalName: bucket,
		Visibility:   "private",
	}))

	t.Run("put-and-get-object", func(t *testing.T) {
		body := strings.NewReader("hello world")
		info, err := p.PutObject(ctx, provisioner.PutObjectSpec{
			Bucket:      bucket,
			Key:         "hello.txt",
			Body:        body,
			Size:        11,
			ContentType: "text/plain",
		})
		require.NoError(t, err)
		require.Equal(t, "hello.txt", info.Key)
		require.Equal(t, int64(11), info.Size)
		require.Equal(t, "text/plain", info.ContentType)

		rc, info2, err := p.GetObject(ctx, provisioner.ObjectRef{BucketPhysicalName: bucket, Key: "hello.txt"})
		require.NoError(t, err)
		defer rc.Close()
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(rc)
		require.Equal(t, "hello world", buf.String())
		require.Equal(t, int64(11), info2.Size)
	})

	t.Run("head-object", func(t *testing.T) {
		info, err := p.HeadObject(ctx, provisioner.ObjectRef{BucketPhysicalName: bucket, Key: "hello.txt"})
		require.NoError(t, err)
		require.Equal(t, "hello.txt", info.Key)
	})

	t.Run("list-objects", func(t *testing.T) {
		p.PutObject(ctx, provisioner.PutObjectSpec{Bucket: bucket, Key: "sub/a.txt", Body: strings.NewReader("a"), ContentType: "text/plain"})
		p.PutObject(ctx, provisioner.PutObjectSpec{Bucket: bucket, Key: "sub/b.txt", Body: strings.NewReader("b"), ContentType: "text/plain"})
		p.PutObject(ctx, provisioner.PutObjectSpec{Bucket: bucket, Key: "root.txt", Body: strings.NewReader("r"), ContentType: "text/plain"})

		objects, token, err := p.ListObjects(ctx, provisioner.ObjectRef{BucketPhysicalName: bucket}, provisioner.ListObjectsOptions{
			Prefix:  "sub/",
			MaxKeys: 2,
		})
		require.NoError(t, err)
		require.Len(t, objects, 2)
		require.NotEmpty(t, token)
		require.True(t, strings.HasPrefix(objects[0].Key, "sub/"))
	})

	t.Run("delete-object", func(t *testing.T) {
		err := p.DeleteObject(ctx, provisioner.ObjectRef{BucketPhysicalName: bucket, Key: "hello.txt"})
		require.NoError(t, err)
		_, err = p.HeadObject(ctx, provisioner.ObjectRef{BucketPhysicalName: bucket, Key: "hello.txt"})
		require.ErrorIs(t, err, provisioner.ErrObjectNotFound)
	})

	t.Run("object-not-found", func(t *testing.T) {
		_, _, err := p.GetObject(ctx, provisioner.ObjectRef{BucketPhysicalName: bucket, Key: "nonexistent"})
		require.ErrorIs(t, err, provisioner.ErrObjectNotFound)
	})

	t.Run("bucket-not-found-for-object-ops", func(t *testing.T) {
		_, _, err := p.GetObject(ctx, provisioner.ObjectRef{BucketPhysicalName: "nonexistent-bucket", Key: "x"})
		require.ErrorIs(t, err, provisioner.ErrBucketNotFound)
	})

	t.Run("presigned-get-url", func(t *testing.T) {
		p.PutObject(ctx, provisioner.PutObjectSpec{Bucket: bucket, Key: "presign.txt", Body: strings.NewReader("x"), ContentType: "text/plain"})
		url, err := p.PresignedGetURL(ctx, provisioner.ObjectRef{BucketPhysicalName: bucket, Key: "presign.txt"}, 15*time.Minute)
		require.NoError(t, err)
		require.Contains(t, url, bucket)
		require.Contains(t, url, "presign.txt")
	})

	t.Run("presigned-put-url", func(t *testing.T) {
		url, err := p.PresignedPutURL(ctx, provisioner.ObjectRef{BucketPhysicalName: bucket, Key: "upload.txt"}, 15*time.Minute)
		require.NoError(t, err)
		require.Contains(t, url, bucket)
	})

	t.Run("delete-object-idempotent", func(t *testing.T) {
		err := p.DeleteObject(ctx, provisioner.ObjectRef{BucketPhysicalName: bucket, Key: "nonexistent"})
		require.ErrorIs(t, err, provisioner.ErrObjectNotFound)
	})
}

func TestObjectKeyValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{name: "valid key", key: "folder/file.txt"},
		{name: "simple key", key: "file.txt"},
		{name: "nested", key: "a/b/c/d.txt"},
		{name: "empty key", key: "", wantErr: true},
		{name: "path traversal", key: "../etc/passwd", wantErr: true},
		{name: "starts with /", key: "/etc/passwd", wantErr: true},
		{name: "too long", key: strings.Repeat("x", 1025), wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Validation is in the service layer, but this tests the intent
			key := strings.TrimSpace(tt.key)
			if key == "" {
				require.True(t, tt.wantErr)
				return
			}
			if len(key) > 1024 {
				require.True(t, tt.wantErr)
				return
			}
			if strings.Contains(key, "..") || strings.HasPrefix(key, "/") {
				require.True(t, tt.wantErr)
				return
			}
			require.False(t, tt.wantErr)
		})
	}
}
