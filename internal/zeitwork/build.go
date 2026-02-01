package zeitwork

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

func (s *Service) reconcileBuild(ctx context.Context, objectID uuid.UUID) error {
	build, err := s.db.Queries.BuildFirstByID(ctx, objectID)
	if err != nil {
		return err
	}

	// reconcile build image for build vm
	image, err := s.db.ImageFindByRepositoryAndTag(ctx, queries.ImageFindByRepositoryAndTagParams{
		Registry:   "ghcr.io",
		Repository: "tomhaerter/dind",
		Tag:        "latest",
	})
	if err == pgx.ErrNoRows {
		slog.Error("build image not found")

		// create build image
		image, err := s.db.ImageCreate(ctx, queries.ImageCreateParams{
			ID:         uuid.New(),
			Registry:   "ghcr.io",
			Repository: "tomhaerter/dind",
			Tag:        "latest",
		})
		if err != nil {
			return err
		}
		slog.Info("created image", "id", image.ID)
		return nil
	}
	if err != nil {
		return err
	}

	if !image.DiskImageKey.Valid {
		slog.Error("disk image of build image not ready")
		return nil
	}

	slog.Info("disk image found", "id", image.ID)

	// if we dont have a vm create one
	if !build.VmID.Valid {
		vm, err := s.VMCreate(ctx, VMCreateParams{
			VCPUs:   2,
			Memory:  4 * 1024,
			ImageID: image.ID,
			Port:    2375,
		})
		if err != nil {
			return err
		}
		return s.db.BuildMarkBuilding(ctx, queries.BuildMarkBuildingParams{
			ID:   build.ID,
			VmID: vm.ID,
		})
	}

	if build.VmID.Valid {
		// check if the vm is ready
		vm, err := s.db.VMFirstByID(ctx, build.VmID)
		if err != nil {
			return err
		}

		// TODO: this ain't properly working as desired.
		// 	vm might be stopped as we are already done with the build.
		// 	need to adjust controlflow
		if vm.Status != queries.VmStatusRunning {
			return errors.New("vm not ready yet")
		}

		// TODO: we should do this at the entry of this reconcile func
		// now that we have a running vm we can check if we have an image
		if !build.ImageID.Valid {
			// if we dont have an image, we now can use the docker in docker vm to create a build
			//
			// what needs to happen
			// 1. download the source code of the user
			// 2. check if we have a Dockerfile
			// 		-> IF NOT then mark build as failed
			// 3. build the repo with the Dockerfile on the remote vm's docker host
			// 4. upload the image to our registry as
			// 		repository = `zeitwork/<base58(project.id)>`
			// 		tag		   = `<deployment.githubCommit>`
			// 5. mark build as successful
			//
			// also we ideally want to stream the docker build logs to the table `build_logs`
		}
	}

	// switch build.Status {
	// case queries.BuildStatusPending:
	// 	panic("unimplemented")
	// 	// -> create a `vm` with status `pending` with the `zeitwork-build` image and update build to status `building`
	// case queries.BuildStatusBuilding:
	// 	panic("unimplemented")
	// 	// -> if build status is `building` for more than 30 minutes mark it as failed
	// 	// -> if vm status `pending`, `starting`, `running` or `stopping` for more than 10 minutes then set build status to `failed`
	// 	// -> if vm status `failed` then set build status to `failed`
	// 	// -> if vm status `stopped` then check build image
	// 	// |-> if it exists then mark build as `successful`
	// 	// |-> if it does not exist then mark build as `failed`
	// case queries.BuildStatusSuccesful:
	// 	panic("unimplemented")
	// case queries.BuildStatusFailed:
	// 	panic("unimplemented")

	// }

	return nil
}
