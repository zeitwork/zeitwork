package zeitwork

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

func (s *Service) reconcileImage(ctx context.Context, objectID uuid.UUID) error {
	// pull max one image at a time
	s.imageMu.Lock()
	defer s.imageMu.Unlock()

	// find image
	image, err := s.db.ImageFindByID(ctx, objectID)
	if err != nil {
		return err
	}

	s3Key := fmt.Sprintf("%s.qcow2", image.ID.String())
	localDiskPath := fmt.Sprintf("/data/base/%s.qcow2", image.ID.String())

	// If the image already has a disk_image_key set, verify the local file exists.
	// If missing, try to download from S3 first. If that also fails, clear and re-pull.
	if image.DiskImageKey.Valid {
		if _, err := os.Stat(localDiskPath); err == nil {
			// Local file exists, we're good
			return nil
		}

		// Local file missing -- try S3
		if s.s3 != nil && s.s3.Exists(ctx, s3Key) {
			slog.Info("base image missing locally, downloading from S3", "id", image.ID, "key", s3Key)
			if err := s.s3.Download(ctx, s3Key, localDiskPath); err != nil {
				slog.Error("failed to download from S3, will re-pull", "id", image.ID, "error", err)
				os.Remove(localDiskPath)
			} else {
				return nil // downloaded successfully
			}
		}

		// Neither local nor S3 -- clear and re-pull
		slog.Error("image does not exist locally or in S3, re-pulling", "id", image.ID)
		err = s.db.ImageUpdateDiskImage(ctx, queries.ImageUpdateDiskImageParams{
			ID:           image.ID,
			DiskImageKey: pgtype.Text{Valid: false},
		})
		if err != nil {
			return err
		}
		s.imageScheduler.Schedule(objectID, time.Now())
		return nil
	}

	// If S3 has the image but we haven't recorded it in DB yet (another server built it),
	// download from S3 and update the DB.
	if s.s3 != nil && s.s3.Exists(ctx, s3Key) {
		slog.Info("image found in S3, downloading", "id", image.ID, "key", s3Key)
		if err := s.s3.Download(ctx, s3Key, localDiskPath); err != nil {
			slog.Error("failed to download from S3", "id", image.ID, "error", err)
		} else {
			// Downloaded successfully -- update DB
			err = s.db.ImageUpdateDiskImage(ctx, queries.ImageUpdateDiskImageParams{
				ID:           image.ID,
				DiskImageKey: pgtype.Text{String: s3Key, Valid: true},
			})
			if err != nil {
				return err
			}
			return nil
		}
	}

	// Need to pull and convert from registry
	tmpdir, err := os.MkdirTemp("", "image")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	// Pull the image using skopeo (with authentication for GHCR)
	imageRef := fmt.Sprintf("%s/%s:%s", image.Registry, image.Repository, image.Tag)
	ociPath := filepath.Join(tmpdir, "oci")
	slog.Info("pulling container image", "ref", imageRef)

	srcCreds := fmt.Sprintf("%s:%s", s.cfg.DockerRegistryUsername, s.cfg.DockerRegistryPAT)
	err = s.runCommand("skopeo", "copy", "--src-creds", srcCreds, fmt.Sprintf("docker://%s", imageRef), fmt.Sprintf("oci:%s:latest", ociPath))
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	// Use umoci to unpack the OCI image to a runtime bundle (produces config.json + rootfs/)
	bundlePath := filepath.Join(tmpdir, "bundle")
	err = s.runCommand("umoci", "unpack", "--image", ociPath+":latest", bundlePath)
	if err != nil {
		return fmt.Errorf("failed to unpack OCI image: %w", err)
	}
	slog.Info("unpacked OCI image to bundle", "path", bundlePath)

	// Remove any existing base image from previous failed attempts (virt-make-fs fails if file exists)
	_ = os.Remove(localDiskPath)

	// Pack the bundle as qcow2 (contains /config.json and /rootfs/ that initagent expects)
	err = s.runCommand("virt-make-fs", "--format=qcow2", "--type=ext4", bundlePath, "--size=+5G", localDiskPath)
	if err != nil {
		slog.Error("oh no! virt-make-fs crashed", "err", err)
		return err
	}

	// Upload to S3 so other servers can download it
	if s.s3 != nil {
		slog.Info("uploading disk image to S3", "id", image.ID, "key", s3Key)
		if err := s.s3.Upload(ctx, s3Key, localDiskPath); err != nil {
			slog.Error("failed to upload to S3", "id", image.ID, "error", err)
			// Non-fatal: the image is still available locally
		}
	}

	// Record the disk image key (S3 key if S3 is configured, local path otherwise)
	diskImageKey := s3Key
	if s.s3 == nil {
		diskImageKey = localDiskPath
	}

	err = s.db.ImageUpdateDiskImage(ctx, queries.ImageUpdateDiskImageParams{
		ID:           image.ID,
		DiskImageKey: pgtype.Text{String: diskImageKey, Valid: true},
	})
	if err != nil {
		slog.Error("failed to upsert disk image", "err", err)
		return err
	}

	return nil
}
