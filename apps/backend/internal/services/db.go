package services

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zeitwork/zeitwork/internal/services/db"
)

//go:generate sqlc generate -f ../../sqlc.yaml

type DB struct {
	*db.Queries
	pool *pgxpool.Pool
}

func NewDB() (*DB, error) {
	// PostgreSQL connection string
	connStr := os.Getenv("POSTGRES_DSN")

	// Create connection pool
	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test the connection
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Initialize SQLC queries
	queries := db.New(pool)
	log.Println("Successfully connected to PostgreSQL database")

	return &DB{
		Queries: queries,
		pool:    pool,
	}, nil
}

func (d *DB) CreateUserAndDefaultOrg(ctx context.Context, username string, userId int64) (db.User, error) {
	user, err := d.UserInsert(ctx, db.UserInsertParams{
		Username: username,
		GithubID: userId,
	})
	if err != nil {
		return db.User{}, err
	}

	// create default org
	organisation, err := d.OrganisationInsert(ctx, db.OrganisationInsertParams{
		Slug: username,
	})
	if err != nil {
		return db.User{}, err
	}

	_, err = d.UserInOrganisationInsert(ctx, db.UserInOrganisationInsertParams{
		UserID:         user.ID,
		OrganisationID: organisation.ID,
	})
	if err != nil {
		return db.User{}, err
	}

	return user, nil
}
