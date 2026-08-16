package provisioner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var _ ObjectStorageProvider = (*S3StorageProvider)(nil)

// S3StorageConfig holds connection parameters for an S3-compatible backend.
type S3StorageConfig struct {
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
}

func (c S3StorageConfig) validate() error {
	if c.Endpoint == "" {
		return errors.New("storage S3 endpoint is required")
	}
	if c.Region == "" {
		return errors.New("storage S3 region is required")
	}
	if c.AccessKeyID == "" {
		return errors.New("storage S3 access key ID is required")
	}
	if c.SecretAccessKey == "" {
		return errors.New("storage S3 secret access key is required")
	}
	return nil
}

// S3StorageProvider implements ObjectStorageProvider using the AWS S3 SDK.
type S3StorageProvider struct {
	client  *s3.Client
	presign *s3.PresignClient
}

// NewS3StorageProvider creates an S3-compatible storage client.
func NewS3StorageProvider(ctx context.Context, cfg S3StorageConfig) (*S3StorageProvider, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		),
		// Default checksum behavior (WhenSupported) makes PutObject compute a
		// CRC checksum for every upload; for unseekable request bodies over a
		// plain-HTTP internal endpoint that fails with "unseekable stream is
		// not supported". Compute checksums only when an operation requires
		// one.
		awsconfig.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = cfg.UsePathStyle
	})

	return &S3StorageProvider{client: client, presign: s3.NewPresignClient(client)}, nil
}

// CreateBucket creates a new bucket using the S3 API.
func (p *S3StorageProvider) CreateBucket(ctx context.Context, spec BucketSpec) error {
	_, err := p.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(spec.PhysicalName),
	})
	if err != nil {
		return mapCreateBucketError(err)
	}
	return nil
}

// DeleteBucket removes a bucket using the S3 API.
func (p *S3StorageProvider) DeleteBucket(ctx context.Context, ref BucketRef) error {
	_, err := p.client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(ref.PhysicalName),
	})
	if err != nil {
		return mapDeleteBucketError(err)
	}
	return nil
}

// BucketExists checks whether a bucket exists using the S3 HeadBucket API.
func (p *S3StorageProvider) BucketExists(ctx context.Context, ref BucketRef) (bool, error) {
	_, err := p.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(ref.PhysicalName),
	})
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// --- Object operations ---

// PutObject uploads an object to the S3 bucket.
// seekableBody returns a body the S3 signer can rewind. Over plain-HTTP
// endpoints SigV4 must hash the payload before sending, which requires a
// seekable stream; HTTP request bodies are not. Spool to a temp file instead
// of buffering in memory so large uploads stay bounded by disk, not RAM.
func seekableBody(body io.Reader) (io.ReadSeeker, func(), error) {
	if rs, ok := body.(io.ReadSeeker); ok {
		return rs, func() {}, nil
	}
	f, err := os.CreateTemp("", "oc-s3-spool-*")
	if err != nil {
		return nil, nil, fmt.Errorf("spool upload body: %w", err)
	}
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}
	if _, err := io.Copy(f, body); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("buffer upload body: %w", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("rewind upload body: %w", err)
	}
	return f, cleanup, nil
}

func (p *S3StorageProvider) PutObject(ctx context.Context, spec PutObjectSpec) (*ObjectInfo, error) {
	body, cleanup, err := seekableBody(spec.Body)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	input := &s3.PutObjectInput{
		Bucket:      aws.String(spec.Bucket),
		Key:         aws.String(spec.Key),
		Body:        body,
		ContentType: aws.String(spec.ContentType),
	}
	if spec.Size > 0 {
		input.ContentLength = aws.Int64(spec.Size)
	}
	out, err := p.client.PutObject(ctx, input)
	if err != nil {
		return nil, err
	}
	return &ObjectInfo{
		Key:          spec.Key,
		Size:         spec.Size,
		ContentType:  spec.ContentType,
		ETag:         deref(out.ETag),
		LastModified: time.Now().UTC(),
	}, nil
}

// GetObject retrieves an object from the S3 bucket.
func (p *S3StorageProvider) GetObject(ctx context.Context, ref ObjectRef) (io.ReadCloser, *ObjectInfo, error) {
	out, err := p.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(ref.BucketPhysicalName),
		Key:    aws.String(ref.Key),
	})
	if err != nil {
		return nil, nil, mapGetObjectError(err)
	}
	info := &ObjectInfo{
		Key:          ref.Key,
		Size:         derefInt(out.ContentLength),
		ContentType:  deref(out.ContentType),
		ETag:         deref(out.ETag),
		LastModified: derefTime(out.LastModified),
	}
	return out.Body, info, nil
}

// ListObjects lists objects in the S3 bucket with the given prefix and options.
func (p *S3StorageProvider) ListObjects(ctx context.Context, ref ObjectRef, opts ListObjectsOptions) ([]ObjectInfo, string, error) {
	input := &s3.ListObjectsV2Input{
		Bucket:            aws.String(ref.BucketPhysicalName),
		Prefix:            aws.String(opts.Prefix),
		MaxKeys:           aws.Int32(opts.MaxKeys),
		ContinuationToken: aws.String(opts.ContinuationToken),
	}
	out, err := p.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, "", err
	}
	objects := make([]ObjectInfo, 0, len(out.Contents))
	for _, o := range out.Contents {
		objects = append(objects, ObjectInfo{
			Key:          deref(o.Key),
			Size:         derefInt(o.Size),
			ContentType:  "",
			ETag:         deref(o.ETag),
			LastModified: derefTime(o.LastModified),
		})
	}
	nextToken := ""
	if out.IsTruncated != nil && *out.IsTruncated {
		nextToken = deref(out.NextContinuationToken)
	}
	return objects, nextToken, nil
}

// DeleteObject removes an object from the S3 bucket.
func (p *S3StorageProvider) DeleteObject(ctx context.Context, ref ObjectRef) error {
	_, err := p.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(ref.BucketPhysicalName),
		Key:    aws.String(ref.Key),
	})
	if err != nil {
		return mapGetObjectError(err)
	}
	return nil
}

// HeadObject returns metadata for an object in the S3 bucket.
func (p *S3StorageProvider) HeadObject(ctx context.Context, ref ObjectRef) (*ObjectInfo, error) {
	out, err := p.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(ref.BucketPhysicalName),
		Key:    aws.String(ref.Key),
	})
	if err != nil {
		return nil, mapGetObjectError(err)
	}
	return &ObjectInfo{
		Key:          ref.Key,
		Size:         derefInt(out.ContentLength),
		ContentType:  deref(out.ContentType),
		ETag:         deref(out.ETag),
		LastModified: derefTime(out.LastModified),
	}, nil
}

// PresignedGetURL generates a presigned URL for downloading an object.
func (p *S3StorageProvider) PresignedGetURL(ctx context.Context, ref ObjectRef, expiry time.Duration) (string, error) {
	req, err := p.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(ref.BucketPhysicalName),
		Key:    aws.String(ref.Key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// PresignedPutURL generates a presigned URL for uploading an object.
func (p *S3StorageProvider) PresignedPutURL(ctx context.Context, ref ObjectRef, expiry time.Duration) (string, error) {
	req, err := p.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(ref.BucketPhysicalName),
		Key:    aws.String(ref.Key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// --- Helpers ---

func mapCreateBucketError(err error) error {
	var bae *s3types.BucketAlreadyExists
	if errors.As(err, &bae) {
		return ErrBucketExists
	}
	var baoy *s3types.BucketAlreadyOwnedByYou
	if errors.As(err, &baoy) {
		return ErrBucketExists
	}
	return err
}

func mapDeleteBucketError(err error) error {
	var nsb *s3types.NoSuchBucket
	if errors.As(err, &nsb) {
		return ErrBucketNotFound
	}
	msg := err.Error()
	if strings.Contains(msg, "BucketNotEmpty") || strings.Contains(msg, "409") {
		return BucketNotEmptyError{Count: nil}
	}
	return err
}

func mapGetObjectError(err error) error {
	if isNotFoundError(err) {
		return ErrObjectNotFound
	}
	return err
}

func isNotFoundError(err error) bool {
	var nsb *s3types.NoSuchBucket
	if errors.As(err, &nsb) {
		return true
	}
	var nsk *s3types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "404") || strings.Contains(msg, "Not Found") || strings.Contains(msg, "NoSuchBucket") || strings.Contains(msg, "NoSuchKey")
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func derefInt(s *int64) int64 {
	if s == nil {
		return 0
	}
	return *s
}
