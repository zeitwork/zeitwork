package graph

import (
	"context"
	"errors"
	"github.com/zeitwork/zeitwork/api/v1alpha1"
	"github.com/zeitwork/zeitwork/internal/auth"
	"github.com/zeitwork/zeitwork/internal/graph/model"
	"github.com/zeitwork/zeitwork/internal/services/db"
	"strconv"
)

func (r *Resolver) GetUser(ctx context.Context) (db.User, error) {
	userID, ok := auth.GetUserIDFromContext(ctx)
	if !ok {
		return db.User{}, errors.New("no user")
	}

	user, err := r.Services.DB.UserFindByID(ctx, userID)
	if err != nil {
		return db.User{}, err
	}

	return user, nil

}

func (r *Resolver) GetUserAndOrg(ctx context.Context, orgId int32) (db.User, db.Organisation, error) {
	user, err := r.GetUser(ctx)
	if err != nil {
		return db.User{}, db.Organisation{}, err
	}

	org, err := r.Services.DB.OrganisationFindWithUser(ctx, db.OrganisationFindWithUserParams{
		UserID: user.ID,
		ID:     orgId,
	})
	if err != nil {
		return db.User{}, db.Organisation{}, err
	}

	return user, org, nil
}

func (r *Resolver) AppToProject(app v1alpha1.App) model.Project {
	id, _ := strconv.Atoi(app.Labels["zeitwork.com/organisationId"])
	return model.Project{
		ID:             app.Namespace + "/" + app.Name,
		K8sName:        app.Name,
		Name:           app.Spec.Description,
		Slug:           app.Spec.Description,
		OrganisationID: int32(id),
	}
}
