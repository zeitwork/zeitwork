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

	diskImageKey := fmt.Sprintf("/data/base/%s.qcow2", image.ID.String())

	// if the image is not valid then we reschedule
	if image.DiskImageKey.Valid {
		_, err = os.Stat(fmt.Sprintf("/data/base/%s.qcow2", image.ID.String()))
		if err != nil {
			slog.Error("image does not exists! but disk image key is set", "id", image.ID)

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

		return nil
	}

	// extract the rootfs
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
	baseImagePath := fmt.Sprintf("/data/base/%s.qcow2", image.ID.String())
	_ = os.Remove(baseImagePath)

	// pack the bundle as a raw ext4 image first, then repair and convert to qcow2.
	// virt-make-fs is known to produce ext4 images with dirty metadata (orphan inode
	// bad checksums), so we run e2fsck before converting to qcow2.
	rawImagePath := baseImagePath + ".raw"
	defer os.Remove(rawImagePath)

	err = s.runCommand("virt-make-fs", "--format=raw", "--type=ext4", bundlePath, "--size=+5G", rawImagePath)
	if err != nil {
		slog.Error("virt-make-fs failed", "err", err)
		return err
	}

	err = s.runCommand("e2fsck", "-fy", rawImagePath)
	if err != nil {
		slog.Error("e2fsck failed", "err", err)
		return err
	}

	err = s.runCommand("qemu-img", "convert", "-f", "raw", "-O", "qcow2", rawImagePath, baseImagePath)
	if err != nil {
		slog.Error("qemu-img convert failed", "err", err)
		return err
	}

	// Upload to S3 for cross-server sharing
	s3Key := fmt.Sprintf("images/%s.qcow2", image.ID.String())
	if err := s.s3.Upload(ctx, s3Key, baseImagePath); err != nil {
		slog.Error("failed to upload image to S3", "image_id", image.ID, "err", err)
		// Don't fail — the image is still available locally. Other servers
		// will retry the download or build the image themselves.
	} else {
		slog.Info("uploaded image to S3", "image_id", image.ID, "s3_key", s3Key)
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
