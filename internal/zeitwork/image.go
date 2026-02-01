package zeitwork

import (
	"context"
	"errors"

	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

func (s *Service) reconcileImage(ctx context.Context, objectID uuid.UUID) error {
	image, err := s.db.ImageFindByID(ctx, objectID)
	if err != nil {
		return err
	}

	if !image.DiskImageKey.Valid {
		// download the docker image
		// extract the rootfs
		// pack the rootfs as qcow2
		// create work disk
		// upload disk image to s3 storage
		// update `disk_image_key` in database
		return errors.New("unimplemented")
	}

	return nil
}

// download docker image
