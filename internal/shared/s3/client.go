package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Client represents an S3 client for storing VM images
type Client struct {
	client *s3.Client
	bucket string
	prefix string
	logger *slog.Logger
}

// Config holds S3 configuration
type Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
	Prefix          string // Optional prefix for all keys
}

// NewClient creates a new S3 client
func NewClient(cfg *Config, logger *slog.Logger) (*Client, error) {
	// Create AWS config
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		)),
		config.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				if cfg.Endpoint != "" {
					return aws.Endpoint{
						URL:               cfg.Endpoint,
						SigningRegion:     cfg.Region,
						HostnameImmutable: true,
					}, nil
				}
				// Use default AWS S3 endpoint
				return aws.Endpoint{}, &aws.EndpointNotFoundError{}
			}),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS config: %w", err)
	}

	// Create S3 client
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true // Required for MinIO and custom S3 endpoints
	})

	// Create client
	c := &Client{
		client: s3Client,
		bucket: cfg.Bucket,
		prefix: cfg.Prefix,
		logger: logger,
	}

	// Ensure bucket exists
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := c.ensureBucket(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure bucket: %w", err)
	}

	return c, nil
}

// ensureBucket ensures the bucket exists, creating it if necessary
func (c *Client) ensureBucket(ctx context.Context) error {
	// Check if bucket exists
	_, err := c.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(c.bucket),
	})

	if err != nil {
		// Try to create bucket
		_, createErr := c.client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(c.bucket),
		})
		if createErr != nil {
			// Check if bucket already exists (race condition)
			_, headErr := c.client.HeadBucket(ctx, &s3.HeadBucketInput{
				Bucket: aws.String(c.bucket),
			})
			if headErr != nil {
				return fmt.Errorf("failed to create bucket: %w", createErr)
			}
		}
		c.logger.Info("Created S3 bucket", "bucket", c.bucket)
	}

	return nil
}

// buildKey builds the full S3 key with optional prefix
func (c *Client) buildKey(key string) string {
	if c.prefix != "" {
		return fmt.Sprintf("%s/%s", strings.TrimSuffix(c.prefix, "/"), strings.TrimPrefix(key, "/"))
	}
	return key
}

// UploadImage uploads a VM image to S3
func (c *Client) UploadImage(ctx context.Context, imageID string, data io.Reader, size int64) error {
	key := c.buildKey(fmt.Sprintf("images/%s.ext4", imageID))

	// Upload to S3
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          data,
		ContentType:   aws.String("application/octet-stream"),
		ContentLength: aws.Int64(size),
		Metadata: map[string]string{
			"image-id":    imageID,
			"upload-time": time.Now().Format(time.RFC3339),
		},
	})

	if err != nil {
		return fmt.Errorf("failed to upload image: %w", err)
	}

	c.logger.Info("Uploaded image to S3", "imageID", imageID, "key", key, "size", size)
	return nil
}

// DownloadImage downloads a VM image from S3
func (c *Client) DownloadImage(ctx context.Context, imageID string, writer io.Writer) error {
	key := c.buildKey(fmt.Sprintf("images/%s.ext4", imageID))

	// Download from S3
	result, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}
	defer result.Body.Close()

	// Copy to writer
	written, err := io.Copy(writer, result.Body)
	if err != nil {
		return fmt.Errorf("failed to write image data: %w", err)
	}

	c.logger.Info("Downloaded image from S3", "imageID", imageID, "key", key, "size", written)
	return nil
}

// DeleteImage deletes a VM image from S3
func (c *Client) DeleteImage(ctx context.Context, imageID string) error {
	key := c.buildKey(fmt.Sprintf("images/%s.ext4", imageID))

	// Delete from S3
	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return fmt.Errorf("failed to delete image: %w", err)
	}

	c.logger.Info("Deleted image from S3", "imageID", imageID, "key", key)
	return nil
}

// ImageExists checks if an image exists in S3
func (c *Client) ImageExists(ctx context.Context, imageID string) (bool, error) {
	key := c.buildKey(fmt.Sprintf("images/%s.ext4", imageID))

	// Check if object exists
	_, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		// Check if it's a not found error
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check image existence: %w", err)
	}

	return true, nil
}

// GetImageMetadata gets metadata for a VM image
func (c *Client) GetImageMetadata(ctx context.Context, imageID string) (map[string]string, error) {
	key := c.buildKey(fmt.Sprintf("images/%s.ext4", imageID))

	// Get object metadata
	result, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get image metadata: %w", err)
	}

	metadata := make(map[string]string)
	for k, v := range result.Metadata {
		metadata[k] = v
	}

	// Add standard metadata
	metadata["content-length"] = fmt.Sprintf("%d", result.ContentLength)
	if result.LastModified != nil {
		metadata["last-modified"] = result.LastModified.Format(time.RFC3339)
	}
	if result.ETag != nil {
		metadata["etag"] = *result.ETag
	}

	return metadata, nil
}

// ListImages lists all VM images in S3
func (c *Client) ListImages(ctx context.Context) ([]string, error) {
	prefix := c.buildKey("images/")

	// List objects with prefix
	result, err := c.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
		Prefix: aws.String(prefix),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}

	var imageIDs []string
	for _, obj := range result.Contents {
		// Extract image ID from key
		key := *obj.Key
		if strings.HasPrefix(key, prefix) && strings.HasSuffix(key, ".ext4") {
			imageID := strings.TrimSuffix(strings.TrimPrefix(key, prefix), ".ext4")
			imageIDs = append(imageIDs, imageID)
		}
	}

	return imageIDs, nil
}

// GeneratePresignedURL generates a presigned URL for downloading an image
func (c *Client) GeneratePresignedURL(ctx context.Context, imageID string, expiration time.Duration) (string, error) {
	key := c.buildKey(fmt.Sprintf("images/%s.ext4", imageID))

	// Create presigned request
	presignClient := s3.NewPresignClient(c.client)
	request, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiration
	})

	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return request.URL, nil
}

// UploadBuildArtifact uploads build artifacts (logs, etc.) to S3
func (c *Client) UploadBuildArtifact(ctx context.Context, buildID string, artifactName string, data []byte) error {
	key := c.buildKey(fmt.Sprintf("builds/%s/%s", buildID, artifactName))

	// Upload to S3
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentType:   aws.String("text/plain"),
		ContentLength: aws.Int64(int64(len(data))),
		Metadata: map[string]string{
			"build-id":    buildID,
			"artifact":    artifactName,
			"upload-time": time.Now().Format(time.RFC3339),
		},
	})

	if err != nil {
		return fmt.Errorf("failed to upload build artifact: %w", err)
	}

	c.logger.Info("Uploaded build artifact to S3", "buildID", buildID, "artifact", artifactName, "key", key)
	return nil
}

// CreateMultipartUpload starts a multipart upload for large images
func (c *Client) CreateMultipartUpload(ctx context.Context, imageID string) (string, error) {
	key := c.buildKey(fmt.Sprintf("images/%s.ext4", imageID))

	result, err := c.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		ContentType: aws.String("application/octet-stream"),
		Metadata: map[string]string{
			"image-id":    imageID,
			"upload-time": time.Now().Format(time.RFC3339),
		},
	})

	if err != nil {
		return "", fmt.Errorf("failed to create multipart upload: %w", err)
	}

	return *result.UploadId, nil
}

// UploadPart uploads a part of a multipart upload
func (c *Client) UploadPart(ctx context.Context, imageID string, uploadID string, partNumber int32, data io.Reader, size int64) (string, error) {
	key := c.buildKey(fmt.Sprintf("images/%s.ext4", imageID))

	result, err := c.client.UploadPart(ctx, &s3.UploadPartInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		UploadId:      aws.String(uploadID),
		PartNumber:    aws.Int32(partNumber),
		Body:          data,
		ContentLength: aws.Int64(size),
	})

	if err != nil {
		return "", fmt.Errorf("failed to upload part %d: %w", partNumber, err)
	}

	return *result.ETag, nil
}

// CompleteMultipartUpload completes a multipart upload
func (c *Client) CompleteMultipartUpload(ctx context.Context, imageID string, uploadID string, parts []types.CompletedPart) error {
	key := c.buildKey(fmt.Sprintf("images/%s.ext4", imageID))

	_, err := c.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(c.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: parts,
		},
	})

	if err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	c.logger.Info("Completed multipart upload", "imageID", imageID, "uploadID", uploadID)
	return nil
}

// AbortMultipartUpload aborts a multipart upload
func (c *Client) AbortMultipartUpload(ctx context.Context, imageID string, uploadID string) error {
	key := c.buildKey(fmt.Sprintf("images/%s.ext4", imageID))

	_, err := c.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(c.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})

	if err != nil {
		return fmt.Errorf("failed to abort multipart upload: %w", err)
	}

	c.logger.Info("Aborted multipart upload", "imageID", imageID, "uploadID", uploadID)
	return nil
}
