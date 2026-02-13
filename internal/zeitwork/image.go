package zeitwork

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

func (s *Service) reconcileImage(ctx context.Context, objectID uuid.UUID) error {
	// Serialize image builds locally (one at a time on this server)
	s.imageMu.Lock()
	defer s.imageMu.Unlock()

	image, err := s.db.ImageFindByID(ctx, objectID)
	if err != nil {
		return err
	}

	baseImagePath := fmt.Sprintf("/data/base/%s.qcow2", image.ID.String())
	s3Key := fmt.Sprintf("images/%s.qcow2", image.ID.String())

	// Image already marked as built in DB -- no need to download here.
	// The VM reconciler pulls from S3 on demand when it needs the image.
	if image.DiskImageKey.Valid {
		if _, err := os.Stat(baseImagePath); err == nil {
			return nil // Already have it locally, nothing to do
		}

		// Not local -- verify it's still in S3. If it is, we're fine (VM
		// reconciler will download when it creates a VM on this server).
		if s.s3 != nil {
			exists, err := s.s3.Exists(ctx, s3Key)
			if err != nil {
				slog.Warn("failed to check image in S3", "image_id", image.ID, "err", err)
			}
			if exists {
				return nil
			}
		}

		// Neither local nor S3 — clear the DB key and fall through to rebuild
		slog.Warn("image missing locally and from S3, clearing DiskImageKey to rebuild", "image_id", image.ID)
		if err := s.db.ImageUpdateDiskImage(ctx, queries.ImageUpdateDiskImageParams{
			ID:           image.ID,
			DiskImageKey: pgtype.Text{Valid: false},
		}); err != nil {
			return err
		}
	}

	// Claim the build in the DB. If another server already has a fresh claim,
	// this returns pgx.ErrNoRows - schedule a retry later to check if the
	// other server finished (DiskImageKey will be set by then).
	_, err = s.db.ImageClaimBuild(ctx, queries.ImageClaimBuildParams{
		ID:         image.ID,
		BuildingBy: s.serverID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		slog.Info("image build claimed by another server, scheduling retry", "image_id", image.ID)
		s.imageScheduler.Schedule(image.ID, time.Now().Add(2*time.Minute))
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to claim image build: %w", err)
	}

	// Build the image locally: pull container image, unpack, create qcow2
	diskImageKey, err := s.buildImage(ctx, image, baseImagePath, s3Key)
	if err != nil {
		// Release the claim on failure so another server (or retry) can pick it up.
		// We release with an empty disk_image_key to just clear building_by.
		_ = s.db.ImageReleaseBuild(ctx, queries.ImageReleaseBuildParams{
			ID:           image.ID,
			DiskImageKey: pgtype.Text{Valid: false},
		})
		return err
	}

	// Release the claim and mark the image as built
	return s.db.ImageReleaseBuild(ctx, queries.ImageReleaseBuildParams{
		ID:           image.ID,
		DiskImageKey: pgtype.Text{String: diskImageKey, Valid: true},
	})
}

// buildImage pulls a container image, unpacks it, and creates a qcow2 disk image.
// Returns the local path to the built image on success.
func (s *Service) buildImage(ctx context.Context, image queries.Image, baseImagePath, s3Key string) (string, error) {
	tmpdir, err := os.MkdirTemp("", "image")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpdir)

	imageRef := fmt.Sprintf("%s/%s:%s", image.Registry, image.Repository, image.Tag)
	ociPath := filepath.Join(tmpdir, "oci")
	slog.Info("building image locally", "ref", imageRef, "image_id", image.ID)

	if strings.Index(imageRef, "ghcr.io/zeitwork") == 0 {
		srcCreds := fmt.Sprintf("%s:%s", s.cfg.DockerRegistryUsername, s.cfg.DockerRegistryPAT)
		err = s.runCommand("skopeo", "copy", "--src-creds", srcCreds, fmt.Sprintf("docker://%s", imageRef), fmt.Sprintf("oci:%s:latest", ociPath))
	} else {
		err = s.runCommand("skopeo", "copy", fmt.Sprintf("docker://%s", imageRef), fmt.Sprintf("oci:%s:latest", ociPath))
	}
	if err != nil {
		return "", fmt.Errorf("failed to pull image: %w", err)
	}

	bundlePath := filepath.Join(tmpdir, "bundle")
	if err := s.runCommand("umoci", "unpack", "--image", ociPath+":latest", bundlePath); err != nil {
		return "", fmt.Errorf("failed to unpack OCI image: %w", err)
	}
	slog.Info("unpacked OCI image to bundle", "path", bundlePath)

	// Remove any existing base image from previous failed attempts (virt-make-fs fails if file exists)
	_ = os.Remove(baseImagePath)

	if err := s.runCommand("virt-make-fs", "--format=qcow2", "--type=ext4", bundlePath, "--size=+5G", baseImagePath); err != nil {
		slog.Error("virt-make-fs failed", "err", err)
		return "", err
	}

	// Upload to S3 so other servers can download instead of rebuilding
	if s.s3 != nil {
		if err := s.s3.Upload(ctx, s3Key, baseImagePath); err != nil {
			return "", fmt.Errorf("failed to upload image to S3: %w", err)
		}
		slog.Info("uploaded image to S3", "image_id", image.ID, "s3_key", s3Key)
	}

	return baseImagePath, nil
}
