package zeitwork

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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

	//// if the image already exists, skip the download step
	//_, err = os.Stat(fmt.Sprintf("/data/base/%s.qcow2", image.ID.String()))
	//if err == nil {
	//	slog.Warn("image already exists!", "id", image.ID)
	//
	//
	//}

	// try to pull the image
	err = s.runCommand("skopeo", "copy", fmt.Sprintf("docker://%s/%s:%s", image.Registry, image.Repository, image.Tag), fmt.Sprintf("oci:%s:latest", image.ID.String()))
	if err != nil {
		return err
	}

	rootfs := tmpdir + "/"
	err = s.runCommand("umoci", "unpack", "--image", image.ID.String()+":latest", rootfs)
	if err != nil {
		return err
	}

	// pack the rootfs as qcow2
	err = s.runCommand("virt-make-fs", "--format=qcow2", "--type=ext4", rootfs, "--size=+5G", fmt.Sprintf("/data/base/%s.qcow2", image.ID.String()))
	if err != nil {
		slog.Error("oh no! virt-make-fs crashed", "err", err)
		return err
	}

	//// skip if the work disk already exists
	//_, err = os.Stat(fmt.Sprintf("/data/work/%s.qcow2", image.ID.String()))
	//if err == nil {
	//	slog.Warn("work image already exists!", "id", image.ID)
	//
	//	err = os.Remove(fmt.Sprintf("/data/work/%s.qcow2", image.ID.String()))
	//	if err != nil {
	//		return err
	//	}
	//
	//	s.imageScheduler.Schedule(objectID, time.Now())
	//	return nil
	//}

	// remove existing
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
