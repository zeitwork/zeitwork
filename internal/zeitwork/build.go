package zeitwork

import (
	"context"

	"github.com/zeitwork/zeitwork/internal/database/queries"
	"github.com/zeitwork/zeitwork/internal/shared/uuid"
)

func (s *Service) reconcileBuild(ctx context.Context, objectID uuid.UUID) error {
	build, err := s.db.Queries.BuildFirstByID(ctx, objectID)
	if err != nil {
		return err
	}

	switch build.Status {
	case queries.BuildStatusPending:
		panic("unimplemented")
		// -> create a `vm` with status `pending` with the `zeitwork-build` image and update build to status `building`
	case queries.BuildStatusBuilding:
		panic("unimplemented")
		// -> if build status is `building` for more than 30 minutes mark it as failed
		// -> if vm status `pending`, `starting`, `running` or `stopping` for more than 10 minutes then set build status to `failed`
		// -> if vm status `failed` then set build status to `failed`
		// -> if vm status `stopped` then check build image
		// |-> if it exists then mark build as `successful`
		// |-> if it does not exist then mark build as `failed`
	case queries.BuildStatusSuccesful:
		panic("unimplemented")
	case queries.BuildStatusFailed:
		panic("unimplemented")

	}

	return nil
}
