// Package provisioner defines the hosting capabilities consumed by services.
// Concrete backends must be idempotent because jobs are retried after ambiguous failures.
package provisioner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrBucketExists is returned when a bucket already exists.
	ErrBucketExists = errors.New("bucket already exists")
	// ErrBucketNotFound is returned when a bucket is not found.
	ErrBucketNotFound = errors.New("bucket not found")
	// ErrObjectNotFound is returned when an object is not found.
	ErrObjectNotFound = errors.New("object not found")
	// ErrInvalidObjectKey is returned when an object key is invalid.
	ErrInvalidObjectKey = errors.New("invalid object key")
	// ErrObjectTooLarge is returned when an object exceeds the maximum size.
	ErrObjectTooLarge = errors.New("object exceeds maximum size")
)

// BucketNotEmptyError is returned when a bucket cannot be deleted because it contains objects.
type BucketNotEmptyError struct {
	Count *int64
}

func (e BucketNotEmptyError) Error() string { return "bucket is not empty" }

// IsBucketNotEmpty reports whether err is a BucketNotEmptyError.
func IsBucketNotEmpty(err error) bool {
	var bucketErr BucketNotEmptyError
	return errors.As(err, &bucketErr)
}

// --- Bucket operations ---

// ObjectStorageProvider defines the interface for object storage operations.
type ObjectStorageProvider interface {
	CreateBucket(ctx context.Context, spec BucketSpec) error
	DeleteBucket(ctx context.Context, ref BucketRef) error
	BucketExists(ctx context.Context, ref BucketRef) (bool, error)

	PutObject(ctx context.Context, spec PutObjectSpec) (*ObjectInfo, error)
	GetObject(ctx context.Context, ref ObjectRef) (io.ReadCloser, *ObjectInfo, error)
	ListObjects(ctx context.Context, ref ObjectRef, opts ListObjectsOptions) ([]ObjectInfo, string, error)
	DeleteObject(ctx context.Context, ref ObjectRef) error
	HeadObject(ctx context.Context, ref ObjectRef) (*ObjectInfo, error)

	PresignedGetURL(ctx context.Context, ref ObjectRef, expiry time.Duration) (string, error)
	PresignedPutURL(ctx context.Context, ref ObjectRef, expiry time.Duration) (string, error)
}

// --- Types ---

// BucketSpec holds the configuration for creating a bucket.
type BucketSpec struct {
	BucketID     uuid.UUID
	AccountID    uuid.UUID
	PhysicalName string
	Visibility   string
}

// BucketRef is a reference to a physical bucket.
type BucketRef struct {
	BucketID     uuid.UUID
	AccountID    uuid.UUID
	PhysicalName string
}

// ObjectRef is a reference to an object within a bucket.
type ObjectRef struct {
	BucketPhysicalName string
	Key                string
}

// PutObjectSpec holds the data for uploading an object.
type PutObjectSpec struct {
	Bucket      string
	Key         string
	Body        io.Reader
	Size        int64
	ContentType string
}

// ListObjectsOptions holds options for listing objects.
type ListObjectsOptions struct {
	Prefix            string
	MaxKeys           int32
	ContinuationToken string
}

// ObjectInfo holds metadata about a stored object.
type ObjectInfo struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
}

// --- FakeStorageProvider ---

// FakeStorageProvider is an in-memory ObjectStorageProvider for testing.
type FakeStorageProvider struct {
	buckets map[string]*FakeBucketState
}

// FakeBucketState holds the state of a fake bucket.
type FakeBucketState struct {
	PhysicalName string
	Visibility   string
	Objects      map[string]*FakeObjectState
}

// FakeObjectState holds the state of a fake object.
type FakeObjectState struct {
	Key         string
	Data        []byte
	ContentType string
	ETag        string
	Modified    time.Time
}

// NewFakeStorageProvider creates a new in-memory fake storage provider.
func NewFakeStorageProvider() *FakeStorageProvider {
	return &FakeStorageProvider{buckets: make(map[string]*FakeBucketState)}
}

func (f *FakeStorageProvider) ensureBucket(physicalName string) *FakeBucketState {
	b, ok := f.buckets[physicalName]
	if !ok {
		b = &FakeBucketState{PhysicalName: physicalName, Objects: make(map[string]*FakeObjectState)}
		f.buckets[physicalName] = b
	}
	return b
}

// CreateBucket creates a new fake bucket.
func (f *FakeStorageProvider) CreateBucket(_ context.Context, spec BucketSpec) error {
	if _, exists := f.buckets[spec.PhysicalName]; exists {
		return ErrBucketExists
	}
	f.buckets[spec.PhysicalName] = &FakeBucketState{
		PhysicalName: spec.PhysicalName,
		Visibility:   spec.Visibility,
		Objects:      make(map[string]*FakeObjectState),
	}
	return nil
}

// DeleteBucket removes a fake bucket.
func (f *FakeStorageProvider) DeleteBucket(_ context.Context, ref BucketRef) error {
	b, exists := f.buckets[ref.PhysicalName]
	if !exists {
		return ErrBucketNotFound
	}
	if len(b.Objects) > 0 {
		count := int64(len(b.Objects))
		return BucketNotEmptyError{Count: &count}
	}
	delete(f.buckets, ref.PhysicalName)
	return nil
}

// BucketExists checks whether a fake bucket exists.
func (f *FakeStorageProvider) BucketExists(_ context.Context, ref BucketRef) (bool, error) {
	_, exists := f.buckets[ref.PhysicalName]
	return exists, nil
}

// PutObject stores an object in the fake bucket.
func (f *FakeStorageProvider) PutObject(_ context.Context, spec PutObjectSpec) (*ObjectInfo, error) {
	b := f.ensureBucket(spec.Bucket)
	data, err := io.ReadAll(spec.Body)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	obj := &FakeObjectState{
		Key:         spec.Key,
		Data:        data,
		ContentType: spec.ContentType,
		ETag:        fakeETag(data),
		Modified:    now,
	}
	b.Objects[spec.Key] = obj
	return &ObjectInfo{
		Key:          spec.Key,
		Size:         int64(len(data)),
		ContentType:  spec.ContentType,
		ETag:         obj.ETag,
		LastModified: now,
	}, nil
}

// GetObject retrieves an object from the fake bucket.
func (f *FakeStorageProvider) GetObject(_ context.Context, ref ObjectRef) (io.ReadCloser, *ObjectInfo, error) {
	b, ok := f.buckets[ref.BucketPhysicalName]
	if !ok {
		return nil, nil, ErrBucketNotFound
	}
	obj, ok := b.Objects[ref.Key]
	if !ok {
		return nil, nil, ErrObjectNotFound
	}
	info := &ObjectInfo{
		Key:          obj.Key,
		Size:         int64(len(obj.Data)),
		ContentType:  obj.ContentType,
		ETag:         obj.ETag,
		LastModified: obj.Modified,
	}
	return io.NopCloser(bytes.NewReader(obj.Data)), info, nil
}

// ListObjects lists objects in the fake bucket.
func (f *FakeStorageProvider) ListObjects(_ context.Context, ref ObjectRef, opts ListObjectsOptions) ([]ObjectInfo, string, error) {
	b, ok := f.buckets[ref.BucketPhysicalName]
	if !ok {
		return nil, "", ErrBucketNotFound
	}
	keys := make([]string, 0, len(b.Objects))
	for k := range b.Objects {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var objects []ObjectInfo
	count := int32(0)
	lastKey := ""
	for _, k := range keys {
		obj := b.Objects[k]
		if opts.Prefix != "" && !strings.HasPrefix(k, opts.Prefix) {
			continue
		}
		if opts.ContinuationToken != "" && k <= opts.ContinuationToken {
			continue
		}
		if opts.MaxKeys > 0 && count >= opts.MaxKeys {
			continue
		}
		objects = append(objects, ObjectInfo{
			Key:          obj.Key,
			Size:         int64(len(obj.Data)),
			ContentType:  obj.ContentType,
			ETag:         obj.ETag,
			LastModified: obj.Modified,
		})
		lastKey = k
		count++
	}
	var nextToken string
	if opts.MaxKeys > 0 && count >= opts.MaxKeys {
		nextToken = lastKey
	}
	return objects, nextToken, nil
}

// DeleteObject removes an object from the fake bucket.
func (f *FakeStorageProvider) DeleteObject(_ context.Context, ref ObjectRef) error {
	b, ok := f.buckets[ref.BucketPhysicalName]
	if !ok {
		return ErrBucketNotFound
	}
	if _, ok := b.Objects[ref.Key]; !ok {
		return ErrObjectNotFound
	}
	delete(b.Objects, ref.Key)
	return nil
}

// HeadObject returns metadata for an object in the fake bucket.
func (f *FakeStorageProvider) HeadObject(_ context.Context, ref ObjectRef) (*ObjectInfo, error) {
	b, ok := f.buckets[ref.BucketPhysicalName]
	if !ok {
		return nil, ErrBucketNotFound
	}
	obj, ok := b.Objects[ref.Key]
	if !ok {
		return nil, ErrObjectNotFound
	}
	return &ObjectInfo{
		Key:          obj.Key,
		Size:         int64(len(obj.Data)),
		ContentType:  obj.ContentType,
		ETag:         obj.ETag,
		LastModified: obj.Modified,
	}, nil
}

// PresignedGetURL generates a fake presigned URL for downloading an object.
func (f *FakeStorageProvider) PresignedGetURL(_ context.Context, ref ObjectRef, _ time.Duration) (string, error) {
	if b, ok := f.buckets[ref.BucketPhysicalName]; ok {
		if _, ok := b.Objects[ref.Key]; ok {
			return "http://fake-presigned.example.com/get/" + ref.BucketPhysicalName + "/" + ref.Key, nil
		}
	}
	return "", ErrObjectNotFound
}

// PresignedPutURL generates a fake presigned URL for uploading an object.
func (f *FakeStorageProvider) PresignedPutURL(_ context.Context, ref ObjectRef, _ time.Duration) (string, error) {
	if _, ok := f.buckets[ref.BucketPhysicalName]; ok {
		return "http://fake-presigned.example.com/put/" + ref.BucketPhysicalName + "/" + ref.Key, nil
	}
	return "", ErrBucketNotFound
}

// SetBucketsForTest replaces the internal bucket map for testing.
func (f *FakeStorageProvider) SetBucketsForTest(buckets map[string]*FakeBucketState) {
	f.buckets = buckets
}

func fakeETag(data []byte) string {
	h := uint32(len(data))
	for _, b := range data {
		h = h*31 + uint32(b)
	}
	return fmt.Sprintf("\"%08x\"", h)
}

var _ ObjectStorageProvider = (*FakeStorageProvider)(nil)
