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

	diskImageKey := fmt.Sprintf("/data/work/%s.qcow2", image.ID.String())

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

	// pack the bundle as qcow2 (contains /config.json and /rootfs/ that initagent expects)
	err = s.runCommand("virt-make-fs", "--format=qcow2", "--type=ext4", bundlePath, "--size=+5G", baseImagePath)
	if err != nil {
		slog.Error("oh no! virt-make-fs crashed", "err", err)
		return err
	}

	// remove existing work image
	_ = os.Remove(diskImageKey)

	err = s.runCommand("qemu-img", "create", "-f", "qcow2", "-b", fmt.Sprintf("/data/base/%s.qcow2", image.ID.String()), "-F", "qcow2", diskImageKey)
	if err != nil {
		return err
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
