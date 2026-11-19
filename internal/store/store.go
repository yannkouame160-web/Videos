// Package store donne accès aux données de l'application.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNomDejaUtilise est renvoyé quand une application porte déjà ce nom.
var ErrNomDejaUtilise = errors.New("nom déjà utilisé")

// ErrApplicationIntrouvable est renvoyé quand l'application demandée n'existe pas.
var ErrApplicationIntrouvable = errors.New("application introuvable")

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
	ID                      string
	Nom                     string
	EstActive               bool
	DateCreation            time.Time
	ModerationActivee       bool
	CreationAutoUtilisateur bool
	DelaiRevueSecondes      *int
	PlafondStockageOctets   *int64
	PlafondNbVideos         *int
	LimiteRequetesParMinute *int
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

// GetApplicationByKeyHash renvoie l'application dont la clé correspond à l'empreinte donnée.
func (s *Store) GetApplicationByKeyHash(ctx context.Context, empreinte string) (Application, error) {
	const requete = `
		SELECT id_application::text, nom, est_active, date_creation,
		       moderation_activee, creation_auto_utilisateur,
		       delai_revue_secondes, plafond_stockage_octets,
		       plafond_nb_videos, limite_requetes_par_minute
		FROM application
		WHERE empreinte_cle = $1`

	var app Application
	err := s.pool.QueryRow(ctx, requete, empreinte).Scan(
		&app.ID,
		&app.Nom,
		&app.EstActive,
		&app.DateCreation,
		&app.ModerationActivee,
		&app.CreationAutoUtilisateur,
		&app.DelaiRevueSecondes,
		&app.PlafondStockageOctets,
		&app.PlafondNbVideos,
		&app.LimiteRequetesParMinute,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Application{}, ErrApplicationIntrouvable
		}
		return Application{}, err
	}
	return app, nil
}

// SetApplicationActive active ou désactive une application. Un identifiant
// non valide comme UUID renvoie ErrApplicationIntrouvable.
func (s *Store) SetApplicationActive(ctx context.Context, id string, active bool) error {
	const requete = `UPDATE application SET est_active = $2 WHERE id_application = $1::uuid`

	tag, err := s.pool.Exec(ctx, requete, id, active)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "22P02" {
			return ErrApplicationIntrouvable
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrApplicationIntrouvable
	}
	return nil
}
