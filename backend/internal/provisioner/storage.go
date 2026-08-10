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
	ErrBucketExists     = errors.New("bucket already exists")
	ErrBucketNotFound   = errors.New("bucket not found")
	ErrObjectNotFound   = errors.New("object not found")
	ErrInvalidObjectKey = errors.New("invalid object key")
	ErrObjectTooLarge   = errors.New("object exceeds maximum size")
)

type BucketNotEmptyError struct {
	Count *int64
}

func (e BucketNotEmptyError) Error() string { return "bucket is not empty" }

func IsBucketNotEmpty(err error) bool {
	var bucketErr BucketNotEmptyError
	return errors.As(err, &bucketErr)
}

// --- Bucket operations ---

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

type BucketSpec struct {
	BucketID     uuid.UUID
	AccountID    uuid.UUID
	PhysicalName string
	Visibility   string
}

type BucketRef struct {
	BucketID     uuid.UUID
	AccountID    uuid.UUID
	PhysicalName string
}

type ObjectRef struct {
	BucketPhysicalName string
	Key                string
}

type PutObjectSpec struct {
	Bucket      string
	Key         string
	Body        io.Reader
	Size        int64
	ContentType string
}

type ListObjectsOptions struct {
	Prefix            string
	MaxKeys           int32
	ContinuationToken string
}

type ObjectInfo struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
}

// --- FakeStorageProvider ---

type FakeStorageProvider struct {
	buckets map[string]*FakeBucketState
}

type FakeBucketState struct {
	PhysicalName string
	Visibility   string
	Objects      map[string]*FakeObjectState
}

type FakeObjectState struct {
	Key         string
	Data        []byte
	ContentType string
	ETag        string
	Modified    time.Time
}

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

func (f *FakeStorageProvider) BucketExists(_ context.Context, ref BucketRef) (bool, error) {
	_, exists := f.buckets[ref.PhysicalName]
	return exists, nil
}

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

func (f *FakeStorageProvider) PresignedGetURL(_ context.Context, ref ObjectRef, _ time.Duration) (string, error) {
	if b, ok := f.buckets[ref.BucketPhysicalName]; ok {
		if _, ok := b.Objects[ref.Key]; ok {
			return "http://fake-presigned.example.com/get/" + ref.BucketPhysicalName + "/" + ref.Key, nil
		}
	}
	return "", ErrObjectNotFound
}

func (f *FakeStorageProvider) PresignedPutURL(_ context.Context, ref ObjectRef, _ time.Duration) (string, error) {
	if _, ok := f.buckets[ref.BucketPhysicalName]; ok {
		return "http://fake-presigned.example.com/put/" + ref.BucketPhysicalName + "/" + ref.Key, nil
	}
	return "", ErrBucketNotFound
}

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
