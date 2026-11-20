package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"unicode/utf8"

	"keyavi-videos/internal/store"
)

// UtilisateurStore est la partie du store utilisée pour les utilisateurs externes.
type UtilisateurStore interface {
	GetUtilisateur(ctx context.Context, idApp, idExterne string) (store.Utilisateur, error)
	ObtenirOuCreerUtilisateur(ctx context.Context, idApp, idExterne string) (store.Utilisateur, bool, error)
	DefinirRole(ctx context.Context, idApp, idExterne, role string) (store.Utilisateur, error)
	SupprimerUtilisateur(ctx context.Context, idApp, idExterne string, supprimerVideos bool) error
}

// estIdentifiantExterneValide indique si idExterne est non vide, d'au plus 128
// caractères (compte les runes) et sans caractère de contrôle.
func estIdentifiantExterneValide(idExterne string) bool {
	if idExterne == "" {
		return false
	}
	if utf8.RuneCountInString(idExterne) > 128 {
		return false
	}
	for _, r := range idExterne {
		if r < 0x20 || r == 0x7F {
			return false
		}
	}
	return true
}

// repondreUtilisateur renvoie les données publiques d'un utilisateur externe.
func repondreUtilisateur(w http.ResponseWriter, statut int, u store.Utilisateur) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(statut)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id_externe":               u.IDExterne,
		"role":                     u.Role,
		"date_premiere_apparition": u.DatePremiereApparition,
	})
}

// getUtilisateur : GET /v1/utilisateurs/{id_externe}
func getUtilisateur(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	app, ok := ApplicationDepuisContexte(r.Context())
	if !ok {
		ecrireErreur(w, http.StatusUnauthorized, "cle_invalide", messageCleInvalide)
		return
	}
	idExterne := r.PathValue("id_externe")
	if !estIdentifiantExterneValide(idExterne) {
		ecrireErreur(w, http.StatusBadRequest, "identifiant_invalide", "identifiant invalide")
		return
	}
	if deps.Utilisateurs == nil {
		journaliserErreur("GET /v1/utilisateurs/{id_externe}", errors.New("store utilisateurs non configuré"))
		ecrireErreur(w, http.StatusInternalServerError, "erreur_interne", "erreur interne")
		return
	}

	if app.CreationAutoUtilisateur {
		u, cree, err := deps.Utilisateurs.ObtenirOuCreerUtilisateur(r.Context(), app.ID, idExterne)
		if err != nil {
			if errors.Is(err, store.ErrUtilisateurIntrouvable) {
				ecrireErreur(w, http.StatusNotFound, "utilisateur_introuvable", "utilisateur introuvable")
				return
			}
			journaliserErreur("GET /v1/utilisateurs/{id_externe}", err)
			ecrireErreur(w, http.StatusInternalServerError, "erreur_interne", "erreur interne")
			return
		}
		if cree {
			repondreUtilisateur(w, http.StatusCreated, u)
		} else {
			repondreUtilisateur(w, http.StatusOK, u)
		}
		return
	}

	u, err := deps.Utilisateurs.GetUtilisateur(r.Context(), app.ID, idExterne)
	if err != nil {
		if errors.Is(err, store.ErrUtilisateurIntrouvable) {
			ecrireErreur(w, http.StatusNotFound, "utilisateur_introuvable", "utilisateur introuvable")
			return
		}
		journaliserErreur("GET /v1/utilisateurs/{id_externe}", err)
		ecrireErreur(w, http.StatusInternalServerError, "erreur_interne", "erreur interne")
		return
	}
	repondreUtilisateur(w, http.StatusOK, u)
}

// putRoleUtilisateur : PUT /v1/utilisateurs/{id_externe}/role
func putRoleUtilisateur(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	app, ok := ApplicationDepuisContexte(r.Context())
	if !ok {
		ecrireErreur(w, http.StatusUnauthorized, "cle_invalide", messageCleInvalide)
		return
	}
	idExterne := r.PathValue("id_externe")
	if !estIdentifiantExterneValide(idExterne) {
		ecrireErreur(w, http.StatusBadRequest, "identifiant_invalide", "identifiant invalide")
		return
	}
	if deps.Utilisateurs == nil {
		journaliserErreur("PUT /v1/utilisateurs/{id_externe}/role", errors.New("store utilisateurs non configuré"))
		ecrireErreur(w, http.StatusInternalServerError, "erreur_interne", "erreur interne")
		return
	}

	corps := http.MaxBytesReader(w, r.Body, tailleMaxCorps)
	dec := json.NewDecoder(corps)
	dec.DisallowUnknownFields()
	var req struct {
		Role string `json:"role"`
	}
	if err := dec.Decode(&req); err != nil {
		ecrireErreur(w, http.StatusBadRequest, "corps_invalide", "corps JSON invalide")
		return
	}
	if !store.RoleValide(req.Role) {
		ecrireErreur(w, http.StatusBadRequest, "role_invalide", "rôle invalide")
		return
	}

	u, err := deps.Utilisateurs.DefinirRole(r.Context(), app.ID, idExterne, req.Role)
	if err != nil {
		journaliserErreur("PUT /v1/utilisateurs/{id_externe}/role", err)
		ecrireErreur(w, http.StatusInternalServerError, "erreur_interne", "erreur interne")
		return
	}
	repondreUtilisateur(w, http.StatusOK, u)
}

// deleteUtilisateur : DELETE /v1/utilisateurs/{id_externe}
func deleteUtilisateur(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	app, ok := ApplicationDepuisContexte(r.Context())
	if !ok {
		ecrireErreur(w, http.StatusUnauthorized, "cle_invalide", messageCleInvalide)
		return
	}
	idExterne := r.PathValue("id_externe")
	if !estIdentifiantExterneValide(idExterne) {
		ecrireErreur(w, http.StatusBadRequest, "identifiant_invalide", "identifiant invalide")
		return
	}
	if deps.Utilisateurs == nil {
		journaliserErreur("DELETE /v1/utilisateurs/{id_externe}", errors.New("store utilisateurs non configuré"))
		ecrireErreur(w, http.StatusInternalServerError, "erreur_interne", "erreur interne")
		return
	}

	supprimerVideos := false
	if _, present := r.URL.Query()["supprimer_videos"]; present {
		switch r.URL.Query().Get("supprimer_videos") {
		case "true":
			supprimerVideos = true
		case "false":
			supprimerVideos = false
		default:
			ecrireErreur(w, http.StatusBadRequest, "parametre_invalide", "paramètre invalide")
			return
		}
	}

	if err := deps.Utilisateurs.SupprimerUtilisateur(r.Context(), app.ID, idExterne, supprimerVideos); err != nil {
		journaliserErreur("DELETE /v1/utilisateurs/{id_externe}", err)
		ecrireErreur(w, http.StatusInternalServerError, "erreur_interne", "erreur interne")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
