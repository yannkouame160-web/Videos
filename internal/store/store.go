// Package store donne accès aux données de l'application.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNomDejaUtilise est renvoyé quand une application porte déjà ce nom.
var ErrNomDejaUtilise = errors.New("nom déjà utilisé")

// Store donne accès à la base de données.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore construit un Store à partir d'un pool de connexions.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// NewApplication contient les données nécessaires à la création d'une application.
type NewApplication struct {
	Nom                     string
	EmpreinteCle            string
	PlafondStockageOctets   int64
	PlafondNbVideos         int
	LimiteRequetesParMinute int
}

// Application est une application telle que renvoyée par la base.
type Application struct {
	ID           string
	Nom          string
	EstActive    bool
	DateCreation time.Time
}

// CreateApplication insère une application et renvoie ses données publiques.
func (s *Store) CreateApplication(ctx context.Context, a NewApplication) (Application, error) {
	const requete = `
		INSERT INTO application (
			nom, empreinte_cle, plafond_stockage_octets,
			plafond_nb_videos, limite_requetes_par_minute
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING id_application::text, nom, est_active, date_creation`

	var app Application
	err := s.pool.QueryRow(ctx, requete,
		a.Nom,
		a.EmpreinteCle,
		a.PlafondStockageOctets,
		a.PlafondNbVideos,
		a.LimiteRequetesParMinute,
	).Scan(&app.ID, &app.Nom, &app.EstActive, &app.DateCreation)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "application_nom_key" {
			return Application{}, ErrNomDejaUtilise
		}
		return Application{}, err
	}
	return app, nil
}
