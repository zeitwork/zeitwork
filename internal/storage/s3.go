package storage

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config holds the configuration for S3/MinIO storage.
type S3Config struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

// S3 provides shared object storage for disk images across servers.
type S3 struct {
	client *minio.Client
	bucket string
}

// NewS3 creates a new S3 client and ensures the bucket exists.
func NewS3(cfg S3Config) (*S3, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}

	// Ensure bucket exists
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket existence: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("failed to create bucket %s: %w", cfg.Bucket, err)
		}
		slog.Info("created S3 bucket", "bucket", cfg.Bucket)
	}

	return &S3{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

// Upload uploads a local file to S3.
func (s *S3) Upload(ctx context.Context, key string, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file for upload: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file for upload: %w", err)
	}

	_, err = s.client.PutObject(ctx, s.bucket, key, f, stat.Size(), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	slog.Info("uploaded file to S3", "key", key, "size", stat.Size())
	return nil
}

// Download downloads an object from S3 to a local file.
func (s *S3) Download(ctx context.Context, key string, destPath string) error {
	err := s.client.FGetObject(ctx, s.bucket, key, destPath, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to download from S3: %w", err)
	}

	slog.Info("downloaded file from S3", "key", key, "dest", destPath)
	return nil
}

// Exists checks if an object exists in S3.
func (s *S3) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == minio.NoSuchKey {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat S3 object: %w", err)
	}
	return true, nil
}
