package provisioner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var _ ObjectStorageProvider = (*S3StorageProvider)(nil)

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

type S3StorageProvider struct {
	client *s3.Client
}

func NewS3StorageProvider(ctx context.Context, cfg S3StorageConfig) (*S3StorageProvider, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	resolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, _ ...any) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               cfg.Endpoint,
				SigningRegion:     cfg.Region,
				HostnameImmutable: true,
			}, nil
		},
	)

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		),
		awsconfig.WithEndpointResolverWithOptions(resolver),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.UsePathStyle
	})

	return &S3StorageProvider{client: client}, nil
}

func (p *S3StorageProvider) CreateBucket(ctx context.Context, spec BucketSpec) error {
	_, err := p.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(spec.PhysicalName),
	})
	if err != nil {
		return mapCreateBucketError(err)
	}
	return nil
}

func (p *S3StorageProvider) DeleteBucket(ctx context.Context, ref BucketRef) error {
	_, err := p.client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(ref.PhysicalName),
	})
	if err != nil {
		return mapDeleteBucketError(err)
	}
	return nil
}

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

func isNotFoundError(err error) bool {
	var nsb *s3types.NoSuchBucket
	if errors.As(err, &nsb) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "404") || strings.Contains(msg, "Not Found") || strings.Contains(msg, "NoSuchBucket")
}
