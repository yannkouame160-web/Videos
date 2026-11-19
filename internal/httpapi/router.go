// Package httpapi expose les routes HTTP du service.
package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"keyavi-videos/internal/apikey"
	"keyavi-videos/internal/store"
)

const tailleMaxCorps = 1 << 20 // 1 Mo

// ApplicationStore est la partie du store utilisée par l'API.
type ApplicationStore interface {
	CreateApplication(ctx context.Context, a store.NewApplication) (store.Application, error)
}

// Dependencies regroupe les dépendances et les valeurs par défaut du routeur.
type Dependencies struct {
	Store                    ApplicationStore
	InvitationCode           string
	DefaultStorageQuotaBytes int64
	DefaultMaxVideos         int
	DefaultRequestsPerMinute int
}

// NewRouter crée le routeur HTTP et déclare les routes de l'API.
func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()

	// La route de santé renvoie simplement "ok".
	mux.HandleFunc("GET /sante", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// La route de création d'application est protégée par le code d'invitation.
	mux.HandleFunc("POST /v1/applications", func(w http.ResponseWriter, r *http.Request) {
		creerApplication(w, r, deps)
	})

	return mux
}

// ecrireErreur renvoie une erreur au format JSON attendu par les clients.
func ecrireErreur(w http.ResponseWriter, statut int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statut)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"erreur": map[string]string{"code": code, "message": message},
	})
}

// codeValide compare les empreintes des codes en temps constant.
func codeValide(fourni, attendu string) bool {
	hFourni := sha256.Sum256([]byte(fourni))
	hAttendu := sha256.Sum256([]byte(attendu))
	return subtle.ConstantTimeCompare(hFourni[:], hAttendu[:]) == 1
}

// creerApplication traite la création d'une application et de sa clé.
func creerApplication(w http.ResponseWriter, r *http.Request, deps Dependencies) {
	// Sans code configuré, la création est désactivée.
	if deps.InvitationCode == "" {
		ecrireErreur(w, http.StatusServiceUnavailable, "creation_desactivee",
			"la création d'application est désactivée")
		return
	}

	// On ne distingue pas un en-tête absent d'un mauvais code.
	code := r.Header.Get("X-Invitation-Code")
	if code == "" || !codeValide(code, deps.InvitationCode) {
		ecrireErreur(w, http.StatusUnauthorized, "code_invitation_invalide",
			"code d'invitation invalide")
		return
	}

	if deps.Store == nil {
		ecrireErreur(w, http.StatusInternalServerError, "erreur_interne", "erreur interne")
		return
	}

	// Lecture d'un corps JSON strict, limité à 1 Mo.
	corps := http.MaxBytesReader(w, r.Body, tailleMaxCorps)
	dec := json.NewDecoder(corps)
	dec.DisallowUnknownFields()

	var req struct {
		Nom string `json:"nom"`
	}
	if err := dec.Decode(&req); err != nil {
		ecrireErreur(w, http.StatusBadRequest, "corps_invalide", "corps JSON invalide")
		return
	}

	nom := strings.TrimSpace(req.Nom)
	if nom == "" || utf8.RuneCountInString(nom) > 100 {
		ecrireErreur(w, http.StatusBadRequest, "nom_invalide", "nom invalide")
		return
	}

	// La clé en clair ne quitte jamais cette fonction : seul le hash est transmis au store.
	cle, empreinte, err := apikey.Generate()
	if err != nil {
		ecrireErreur(w, http.StatusInternalServerError, "erreur_interne", "erreur interne")
		return
	}

	app, err := deps.Store.CreateApplication(r.Context(), store.NewApplication{
		Nom:                     nom,
		EmpreinteCle:            empreinte,
		PlafondStockageOctets:   deps.DefaultStorageQuotaBytes,
		PlafondNbVideos:         deps.DefaultMaxVideos,
		LimiteRequetesParMinute: deps.DefaultRequestsPerMinute,
	})
	if err != nil {
		if errors.Is(err, store.ErrNomDejaUtilise) {
			ecrireErreur(w, http.StatusConflict, "nom_deja_utilise", "ce nom est déjà utilisé")
			return
		}
		// Le détail technique n'est ni renvoyé au client, ni écrit dans les logs.
		ecrireErreur(w, http.StatusInternalServerError, "erreur_interne", "erreur interne")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id_application": app.ID,
		"nom":            app.Nom,
		"cle":            cle,
	})
}
