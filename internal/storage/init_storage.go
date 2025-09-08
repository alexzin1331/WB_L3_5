package storage

import (
	"context"
	"fmt"
	"log"

	"L3_5/models"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

func InitDB(cfg *models.Config) (*pgxpool.Pool, error) {
	const op = "storage.initDB"

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Name,
	)

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", op, err)
	}

	// Run migrations
	m, err := migrate.New("file://migrations", dsn)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", op, err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return nil, fmt.Errorf("%s: %v", op, err)
	}

	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return nil, fmt.Errorf("%s: %v", op, err)
	}

	log.Printf("Migrations applied successfully. Version: %d, Dirty: %t", version, dirty)

	return pool, nil
}
