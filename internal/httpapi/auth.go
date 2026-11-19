package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"keyavi-videos/internal/apikey"
	"keyavi-videos/internal/store"
)

// applicationKey est la clé de contexte privée dans laquelle le portier
// stocke l'application authentifiée pour la requête.
type applicationKey struct{}

// ApplicationDepuisContexte renvoie l'application stockée dans le contexte de la
// requête par le portier, et un booléen indiquant sa présence.
func ApplicationDepuisContexte(ctx context.Context) (store.Application, bool) {
	app, ok := ctx.Value(applicationKey{}).(store.Application)
	return app, ok
}

// messageCleInvalide est le seul message renvoyé par le portier sur une
// authentification échouée : il ne distingue jamais les cas.
const messageCleInvalide = "clé invalide"

// portier retourne le middleware protégeant les routes d'application.
func portier(st ApplicationStore) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if st == nil {
				ecrireErreur(w, http.StatusInternalServerError, "erreur_interne", "erreur interne")
				return
			}

			cle, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || cle == "" {
				ecrireErreur(w, http.StatusUnauthorized, "cle_invalide", messageCleInvalide)
				return
			}

			// Rejet rapide des clés hors format attendu, sans interroger la base.
			if !strings.HasPrefix(cle, "kvk_") || utf8.RuneCountInString(cle) != 68 {
				ecrireErreur(w, http.StatusUnauthorized, "cle_invalide", messageCleInvalide)
				return
			}

			app, err := st.GetApplicationByKeyHash(r.Context(), apikey.Hash(cle))
			if err != nil {
				if errors.Is(err, store.ErrApplicationIntrouvable) {
					ecrireErreur(w, http.StatusUnauthorized, "cle_invalide", messageCleInvalide)
					return
				}
				journaliserErreur("GET /v1/application", err)
				ecrireErreur(w, http.StatusInternalServerError, "erreur_interne", "erreur interne")
				return
			}

			if !app.EstActive {
				ecrireErreur(w, http.StatusForbidden, "application_desactivee", "application désactivée")
				return
			}

			ctx := context.WithValue(r.Context(), applicationKey{}, app)
			next(w, r.WithContext(ctx))
		}
	}
}

// applicationDepuisPortier renvoie les données publiques de l'application
// du portier pour GET /v1/application.
func applicationDepuisPortier(w http.ResponseWriter, r *http.Request) {
	app, ok := ApplicationDepuisContexte(r.Context())
	if !ok {
		ecrireErreur(w, http.StatusUnauthorized, "cle_invalide", messageCleInvalide)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id_application": app.ID,
		"nom":            app.Nom,
		"est_active":     app.EstActive,
	})
}
