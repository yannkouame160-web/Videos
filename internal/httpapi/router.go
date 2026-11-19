// Package httpapi expose les routes HTTP du service.
package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"keyavi-videos/internal/apikey"
	"keyavi-videos/internal/store"

	"github.com/jackc/pgx/v5/pgconn"
)

const tailleMaxCorps = 1 << 20 // 1 Mo

// ApplicationStore est la partie du store utilisée par l'API.
type ApplicationStore interface {
	CreateApplication(ctx context.Context, a store.NewApplication) (store.Application, error)
	GetApplicationByKeyHash(ctx context.Context, empreinte string) (store.Application, error)
	SetApplicationActive(ctx context.Context, id string, active bool) error
}

// Dependencies regroupe les dépendances et les valeurs par défaut du routeur.
type Dependencies struct {
	Store                    ApplicationStore
	InvitationCode           string
	AdminCode                string
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

	// La route de lecture de l'application est protégée par le portier.
	mux.HandleFunc("GET /v1/application", portier(deps.Store)(applicationDepuisPortier))

	// Routes d'administration : activation et désactivation.
	mux.HandleFunc("POST /v1/admin/applications/{id}/desactiver",
		func(w http.ResponseWriter, r *http.Request) {
			adminDefinirActivation(w, r, deps, false)
		})
	mux.HandleFunc("POST /v1/admin/applications/{id}/activer",
		func(w http.ResponseWriter, r *http.Request) {
			adminDefinirActivation(w, r, deps, true)
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
		err := errors.New("store non configuré")
		journaliserErreur("POST /v1/applications", err)
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
		journaliserErreur("POST /v1/applications", err)
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
		// Le détail technique n'est pas renvoyé au client ; il est seulement journalisé.
		journaliserErreur("POST /v1/applications", err)
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

// regexpUUID valide un identifiant d'application au format UUID canonique
// (8-4-4-4-12 hexadécimaux séparés par des tirets).
var regexpUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// estUUIDValide indique si id est au format attendu pour un identifiant d'application.
func estUUIDValide(id string) bool {
	return regexpUUID.MatchString(id)
}

// routeActivation renvoie le nom court de la route d'activation pour la journalisation.
func routeActivation(active bool) string {
	if active {
		return "POST /v1/admin/applications/{id}/activer"
	}
	return "POST /v1/admin/applications/{id}/desactiver"
}

// adminDefinirActivation active ou désactive une application via les routes d'administration.
func adminDefinirActivation(w http.ResponseWriter, r *http.Request, deps Dependencies, active bool) {
	route := routeActivation(active)

	// Sans code d'administration configuré, l'opération est désactivée.
	if deps.AdminCode == "" {
		ecrireErreur(w, http.StatusServiceUnavailable, "administration_desactivee",
			"administration désactivée")
		return
	}

	// On ne distingue pas un en-tête absent d'un mauvais code d'administration.
	code := r.Header.Get("X-Admin-Code")
	if code == "" || !codeValide(code, deps.AdminCode) {
		ecrireErreur(w, http.StatusUnauthorized, "code_admin_invalide",
			"code d'administration invalide")
		return
	}

	id := r.PathValue("id")
	if !estUUIDValide(id) {
		ecrireErreur(w, http.StatusBadRequest, "identifiant_invalide", "identifiant invalide")
		return
	}

	if deps.Store == nil {
		journaliserErreur(route, errors.New("store non configuré"))
		ecrireErreur(w, http.StatusInternalServerError, "erreur_interne", "erreur interne")
		return
	}

	if err := deps.Store.SetApplicationActive(r.Context(), id, active); err != nil {
		if errors.Is(err, store.ErrApplicationIntrouvable) {
			ecrireErreur(w, http.StatusNotFound, "application_introuvable", "application introuvable")
			return
		}
		journaliserErreur(route, err)
		ecrireErreur(w, http.StatusInternalServerError, "erreur_interne", "erreur interne")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id_application": id,
		"est_active":     active,
	})
}

// journaliserErreur journalise une erreur technique sans exposer de secret : pour une
// erreur PostgreSQL, seuls le code et le message sont conservés, jamais le Detail.
func journaliserErreur(route string, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		log.Printf("erreur interne sur %s : code %s — %s", route, pgErr.Code, pgErr.Message)
		return
	}
	log.Printf("erreur interne sur %s : %v", route, err)
}
