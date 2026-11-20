package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Rôles de l'utilisateur externe.
const (
	RoleLecteur        = "lecteur"
	RoleEditeur        = "editeur"
	RoleAdministrateur = "administrateur"
)

// ErrUtilisateurIntrouvable est renvoyé quand l'utilisateur externe n'existe pas.
var ErrUtilisateurIntrouvable = errors.New("utilisateur externe introuvable")

// Utilisateur est un utilisateur externe lié à une application.
type Utilisateur struct {
	IDExterne              string
	Role                   string
	DatePremiereApparition time.Time
}

// RoleValide indique si un rôle est l'un des rôles connus.
func RoleValide(role string) bool {
	switch role {
	case RoleLecteur, RoleEditeur, RoleAdministrateur:
		return true
	}
	return false
}

// GetUtilisateur renvoie l'utilisateur externe identifié par idExterne, filtré sur l'application.
func (s *Store) GetUtilisateur(ctx context.Context, idApp, idExterne string) (Utilisateur, error) {
	const requete = `
		SELECT id_externe, role, date_premiere_apparition
		FROM utilisateur_externe
		WHERE id_application = $1::uuid AND id_externe = $2`

	var u Utilisateur
	err := s.pool.QueryRow(ctx, requete, idApp, idExterne).Scan(&u.IDExterne, &u.Role, &u.DatePremiereApparition)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Utilisateur{}, ErrUtilisateurIntrouvable
		}
		return Utilisateur{}, err
	}
	return u, nil
}

// ObtenirOuCreerUtilisateur renvoie l'utilisateur identifié par idExterne, en le
// créant s'il n'existe pas. Le booléen "cree" vaut true si la création a eu lieu.
func (s *Store) ObtenirOuCreerUtilisateur(ctx context.Context, idApp, idExterne string) (Utilisateur, bool, error) {
	const requeteInsertion = `
		INSERT INTO utilisateur_externe (id_application, id_externe, role)
		VALUES ($1::uuid, $2, $3)
		ON CONFLICT (id_application, id_externe) DO NOTHING
		RETURNING id_externe, role, date_premiere_apparition`

	var u Utilisateur
	err := s.pool.QueryRow(ctx, requeteInsertion, idApp, idExterne, RoleLecteur).Scan(&u.IDExterne, &u.Role, &u.DatePremiereApparition)
	if err == nil {
		return u, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Utilisateur{}, false, err
	}

	u, err = s.GetUtilisateur(ctx, idApp, idExterne)
	if err != nil {
		return Utilisateur{}, false, err
	}
	return u, false, nil
}

// DefinirRole crée l'utilisateur s'il n'existe pas, ou modifie son rôle.
func (s *Store) DefinirRole(ctx context.Context, idApp, idExterne, role string) (Utilisateur, error) {
	const requete = `
		INSERT INTO utilisateur_externe (id_application, id_externe, role)
		VALUES ($1::uuid, $2, $3::role_utilisateur)
		ON CONFLICT (id_application, id_externe) DO UPDATE SET role = EXCLUDED.role
		RETURNING id_externe, role, date_premiere_apparition`

	var u Utilisateur
	err := s.pool.QueryRow(ctx, requete, idApp, idExterne, role).Scan(&u.IDExterne, &u.Role, &u.DatePremiereApparition)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Utilisateur{}, ErrUtilisateurIntrouvable
		}
		return Utilisateur{}, err
	}
	return u, nil
}

// SupprimerUtilisateur supprime l'utilisateur externe.
//
// TODO livrable 2 : supprimer aussi les fichiers du stockage S3 des vidéos effacées, AVANT l'appel SQL, car la base ne supprime que les lignes.
func (s *Store) SupprimerUtilisateur(ctx context.Context, idApp, idExterne string, supprimerVideos bool) error {
	const requete = `SELECT supprimer_utilisateur($1::uuid, $2, $3)`
	_, err := s.pool.Exec(ctx, requete, idApp, idExterne, supprimerVideos)
	return err
}
