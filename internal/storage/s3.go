package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config holds the S3/MinIO configuration
type Config struct {
	Endpoint  string // e.g., "minio.internal:9000"
	Bucket    string // e.g., "zeitwork-images"
	AccessKey string
	SecretKey string
	UseSSL    bool
}

// S3 is a simple wrapper around the MinIO client for uploading/downloading disk images
type S3 struct {
	client *minio.Client
	bucket string
}

// New creates a new S3 storage client and ensures the bucket exists
func New(cfg Config) (*S3, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}

	s := &S3{
		client: client,
		bucket: cfg.Bucket,
	}

	// Ensure bucket exists
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
		slog.Info("created S3 bucket", "bucket", cfg.Bucket)
	}

	return s, nil
}

// Upload uploads a local file to S3
func (s *S3) Upload(ctx context.Context, key string, localPath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", localPath, err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", localPath, err)
	}

	_, err = s.client.PutObject(ctx, s.bucket, key, file, stat.Size(), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		return fmt.Errorf("failed to upload %s to S3: %w", key, err)
	}

	slog.Info("uploaded to S3", "key", key, "size", stat.Size())
	return nil
}

// Download downloads a file from S3 to a local path
func (s *S3) Download(ctx context.Context, key string, localPath string) error {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to get S3 object %s: %w", key, err)
	}
	defer obj.Close()

	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file %s: %w", localPath, err)
	}
	defer file.Close()

	written, err := io.Copy(file, obj)
	if err != nil {
		os.Remove(localPath) // cleanup partial download
		return fmt.Errorf("failed to download %s from S3: %w", key, err)
	}

	slog.Info("downloaded from S3", "key", key, "size", written)
	return nil
}

// Exists checks if a key exists in S3
func (s *S3) Exists(ctx context.Context, key string) bool {
	_, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	return err == nil
}
