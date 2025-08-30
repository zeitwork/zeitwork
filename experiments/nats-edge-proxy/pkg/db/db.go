package db

import (
	"context"
	"zeitfun/pkg/db/db"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/samber/do/v2"
)

type DB struct {
	*db.Queries
}

func NewDB(i do.Injector) (*DB, error) {
	dbI, err := pgxpool.New(context.Background(), "host=localhost password=root user=postgres sslmode=disable")
	if err != nil {
		panic(err)
	}

	return &DB{
		Queries: db.New(dbI),
	}, nil
}
