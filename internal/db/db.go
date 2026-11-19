// Package db gère la connexion PostgreSQL et l'application des migrations.
package db

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	errConnexion   = errors.New("connexion à la base de données impossible")
	errMigration   = errors.New("application des migrations impossible")
	errCheminAbs   = errors.New("chemin du dossier de migrations impossible")
	errURLInvalide = errors.New("URL de base de données invalide")
)

// Connect crée un pool de connexions et vérifie qu'il répond. L'URL n'est jamais incluse dans l'erreur.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, errConnexion
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errConnexion
	}
	return pool, nil
}

// urlPgx5 convertit une URL PostgreSQL en URL acceptée par le pilote pgx v5 de golang-migrate.
func urlPgx5(url string) (string, error) {
	switch {
	case strings.HasPrefix(url, "postgres://"):
		return "pgx5://" + strings.TrimPrefix(url, "postgres://"), nil
	case strings.HasPrefix(url, "postgresql://"):
		return "pgx5://" + strings.TrimPrefix(url, "postgresql://"), nil
	default:
		return "", errURLInvalide
	}
}

// nouvelleMigration construit un migrate à partir du dossier de migrations (chemin absolu).
func nouvelleMigration(url, dossier string) (*migrate.Migrate, error) {
	abs, err := filepath.Abs(dossier)
	if err != nil {
		return nil, errCheminAbs
	}

	urlMigrate, err := urlPgx5(url)
	if err != nil {
		return nil, err
	}

	m, err := migrate.New("file://"+abs, urlMigrate)
	if err != nil {
		return nil, errMigration
	}
	return m, nil
}

// MigrerHaut applique toutes les migrations vers le haut ; ErrNoChange est un succès.
func MigrerHaut(url, dossier string) error {
	m, err := nouvelleMigration(url, dossier)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return errMigration
	}
	return nil
}

// MigrerBas applique toutes les migrations vers le bas ; ErrNoChange est un succès.
func MigrerBas(url, dossier string) error {
	m, err := nouvelleMigration(url, dossier)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return errMigration
	}
	return nil
}
