// Package provisioner defines the hosting capabilities consumed by services.
// Concrete backends must be idempotent because jobs are retried after ambiguous failures.
package provisioner

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	// ErrBucketExists is returned when attempting to create a bucket that already exists.
	ErrBucketExists = errors.New("bucket already exists")
	// ErrBucketNotFound is returned when attempting to operate on a non-existent bucket.
	ErrBucketNotFound = errors.New("bucket not found")
)

// BucketNotEmptyError is a typed error returned by DeleteBucket when objects remain.
// Count may be nil if provider cannot report authoritative count.
type BucketNotEmptyError struct {
	Count *int64 // Authoritative count from provider (may be nil)
}

func (e BucketNotEmptyError) Error() string {
	return "bucket is not empty"
}

// IsBucketNotEmpty reports whether err is a BucketNotEmptyError.
func IsBucketNotEmpty(err error) bool {
	var bucketErr BucketNotEmptyError
	return errors.As(err, &bucketErr)
}

// ObjectStorageProvider defines S3-compatible operations for storage buckets.
// Implemented by FakeStorageProvider for testing; real providers deferred to SLICE 2.
type ObjectStorageProvider interface {
	// CreateBucket provisions a new bucket in the backend. Returns nil on success.
	// Returns ErrBucketExists if bucket already exists with expected name (idempotent).
	CreateBucket(ctx context.Context, spec BucketSpec) error

	// DeleteBucket removes a bucket and validates it is empty. Returns typed errors:
	// - BucketNotEmptyError if objects remain (count may be nil)
	// - ErrBucketNotFound if bucket missing
	DeleteBucket(ctx context.Context, ref BucketRef) error

	// BucketExists checks if a bucket exists (for reconciliation).
	BucketExists(ctx context.Context, ref BucketRef) (bool, error)
}

// BucketSpec defines what to create.
type BucketSpec struct {
	BucketID     uuid.UUID
	AccountID    uuid.UUID
	PhysicalName string
	Visibility   string
}

// BucketRef identifies an existing bucket.
type BucketRef struct {
	BucketID     uuid.UUID
	AccountID    uuid.UUID
	PhysicalName string
}

// FakeStorageProvider implements ObjectStorageProvider for unit testing.
// Does NOT call any external services; simulates S3-compatible behavior in-memory.
type FakeStorageProvider struct {
	buckets map[string]*BucketState
}

// BucketState holds the current state of a bucket in the fake provider.
type BucketState struct {
	PhysicalName string
	Visibility   string
	HasObjects   bool
	ObjectCount  int64
}

// NewFakeStorageProvider constructs an in-memory fake provider for testing.
func NewFakeStorageProvider() *FakeStorageProvider {
	return &FakeStorageProvider{
		buckets: make(map[string]*BucketState),
	}
}

// CreateBucket implements ObjectStorageProvider.CreateBucket.
func (f *FakeStorageProvider) CreateBucket(ctx context.Context, spec BucketSpec) error {
	if _, exists := f.buckets[spec.PhysicalName]; exists {
		return ErrBucketExists
	}
	f.buckets[spec.PhysicalName] = &BucketState{
		PhysicalName: spec.PhysicalName,
		Visibility:   spec.Visibility,
		HasObjects:   false,
		ObjectCount:  0,
	}
	return nil
}

// DeleteBucket implements ObjectStorageProvider.DeleteBucket.
func (f *FakeStorageProvider) DeleteBucket(ctx context.Context, ref BucketRef) error {
	bucket, exists := f.buckets[ref.PhysicalName]
	if !exists {
		return ErrBucketNotFound
	}
	if bucket.HasObjects {
		count := int64(bucket.ObjectCount)
		return BucketNotEmptyError{Count: &count}
	}
	delete(f.buckets, ref.PhysicalName)
	return nil
}

// BucketExists implements ObjectStorageProvider.BucketExists.
func (f *FakeStorageProvider) BucketExists(ctx context.Context, ref BucketRef) (bool, error) {
	_, exists := f.buckets[ref.PhysicalName]
	return exists, nil
}

// SetBucketsForTest sets the internal bucket map for testing purposes.
func (f *FakeStorageProvider) SetBucketsForTest(buckets map[string]*BucketState) {
	f.buckets = buckets
}
