package graph

import (
	"context"
	"github.com/google/go-github/v73/github"
	"github.com/zeitwork/zeitwork/internal/services/db"
)

func (r *Resolver) CreateUserAndDefaultOrg(ctx context.Context, ghUser github.User) (db.User, error) {
	user, err := r.Services.DB.UserInsert(ctx, db.UserInsertParams{
		Username: *ghUser.Login,
		GithubID: *ghUser.ID,
	})
	if err != nil {
		return db.User{}, err
	}

	// create default org
	organisation, err := r.Services.DB.OrganisationInsert(ctx, db.OrganisationInsertParams{
		Slug: *ghUser.Login,
	})
	if err != nil {
		return db.User{}, err
	}

	_, err = r.Services.DB.UserInOrganisationInsert(ctx, db.UserInOrganisationInsertParams{
		UserID:         user.ID,
		OrganisationID: organisation.ID,
	})
	if err != nil {
		return db.User{}, err
	}

	return user, nil
}
