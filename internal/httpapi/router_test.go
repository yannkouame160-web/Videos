package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"keyavi-videos/internal/apikey"
	"keyavi-videos/internal/store"
)

func TestSante(t *testing.T) {
	router := NewRouter(Dependencies{})

	req := httptest.NewRequest(http.MethodGet, "/sante", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("code HTTP: attendu 200, obtenu %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("corps: attendu %q, obtenu %q", "ok", rec.Body.String())
	}
}

// fauxStore enregistre ce qu'il reçoit sans jamais toucher à une base de données.
type fauxStore struct {
	recu           store.NewApplication
	err            error             // erreur renvoyée par CreateApplication
	byKey          store.Application // application renvoyée par GetApplicationByKeyHash
	errByKey       error             // erreur renvoyée par GetApplicationByKeyHash
	errActive      error             // erreur renvoyée par SetApplicationActive
	empreinteRecue string            // dernière empreinte reçue par GetApplicationByKeyHash
	callsByKey     int
	idActiveRecu   string // dernier id reçu par SetApplicationActive
	activeRecu     bool   // dernier état reçu par SetApplicationActive
	callsActive    int
}

func (f *fauxStore) CreateApplication(_ context.Context, a store.NewApplication) (store.Application, error) {
	f.recu = a
	if f.err != nil {
		return store.Application{}, f.err
	}
	return store.Application{ID: "id-test", Nom: a.Nom}, nil
}

func (f *fauxStore) GetApplicationByKeyHash(_ context.Context, empreinte string) (store.Application, error) {
	f.empreinteRecue = empreinte
	f.callsByKey++
	if f.errByKey != nil {
		return store.Application{}, f.errByKey
	}
	return f.byKey, nil
}

func (f *fauxStore) SetApplicationActive(_ context.Context, id string, active bool) error {
	f.idActiveRecu = id
	f.activeRecu = active
	f.callsActive++
	return f.errActive
}

func depsAvecCode(f *fauxStore) Dependencies {
	return Dependencies{
		Store:                    f,
		InvitationCode:           "code-invitation-test",
		DefaultStorageQuotaBytes: 1234,
		DefaultMaxVideos:         56,
		DefaultRequestsPerMinute: 78,
	}
}

func depsAvecAdmin(f *fauxStore, codeAdmin string) Dependencies {
	deps := depsAvecCode(f)
	deps.AdminCode = codeAdmin
	return deps
}

func postApplication(t *testing.T, router http.Handler, code string, corps string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/applications", strings.NewReader(corps))
	if code != "" {
		req.Header.Set("X-Invitation-Code", code)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func codeErreur(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var rep struct {
		Erreur struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"erreur"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatalf("réponse JSON invalide: %v (%q)", err, rec.Body.String())
	}
	if rep.Erreur.Code == "" || rep.Erreur.Message == "" {
		t.Errorf("code ou message manquant: %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type inattendu: %q", ct)
	}
	return rep.Erreur.Code
}

func TestCreationSucces(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecCode(f))

	rec := postApplication(t, router, "code-invitation-test", `{"nom":"  App Test  "}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code HTTP: attendu 201, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("en-tête Cache-Control: no-store attendu")
	}

	var rep struct {
		ID  string `json:"id_application"`
		Nom string `json:"nom"`
		Cle string `json:"cle"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatalf("réponse JSON invalide: %v", err)
	}
	if !strings.HasPrefix(rep.Cle, "kvk_") {
		t.Errorf("la clé doit commencer par kvk_, obtenu %q", rep.Cle)
	}
	if rep.Nom != "App Test" {
		t.Errorf("nom: attendu %q, obtenu %q", "App Test", rep.Nom)
	}
	if rep.ID != "id-test" {
		t.Errorf("id: attendu %q, obtenu %q", "id-test", rep.ID)
	}

	if f.recu.EmpreinteCle != apikey.Hash(rep.Cle) {
		t.Error("le store doit recevoir l'empreinte de la clé renvoyée")
	}
	if f.recu.EmpreinteCle == rep.Cle || strings.Contains(f.recu.EmpreinteCle, rep.Cle) {
		t.Error("le store ne doit jamais recevoir la clé en clair")
	}
	if f.recu.Nom != "App Test" {
		t.Errorf("nom transmis au store: attendu %q, obtenu %q", "App Test", f.recu.Nom)
	}
	if f.recu.PlafondStockageOctets != 1234 || f.recu.PlafondNbVideos != 56 || f.recu.LimiteRequetesParMinute != 78 {
		t.Errorf("plafonds par défaut non transmis: %+v", f.recu)
	}
}

func TestCreationSansEnTete(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecCode(f))
	rec := postApplication(t, router, "", `{"nom":"App"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code HTTP: attendu 401, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "code_invitation_invalide" {
		t.Errorf("code d'erreur: attendu code_invitation_invalide, obtenu %q", code)
	}
	if f.recu.Nom != "" {
		t.Error("le store ne doit pas être appelé")
	}
}

func TestCreationMauvaisCode(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecCode(f))
	rec := postApplication(t, router, "mauvais-code", `{"nom":"App"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code HTTP: attendu 401, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "code_invitation_invalide" {
		t.Errorf("code d'erreur: attendu code_invitation_invalide, obtenu %q", code)
	}
}

func TestCreationDesactivee(t *testing.T) {
	f := &fauxStore{}
	deps := depsAvecCode(f)
	deps.InvitationCode = ""
	router := NewRouter(deps)
	rec := postApplication(t, router, "n-importe-quoi", `{"nom":"App"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("code HTTP: attendu 503, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "creation_desactivee" {
		t.Errorf("code d'erreur: attendu creation_desactivee, obtenu %q", code)
	}
}

func TestCreationCorpsInvalide(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecCode(f))
	rec := postApplication(t, router, "code-invitation-test", `{"nom":`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code HTTP: attendu 400, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "corps_invalide" {
		t.Errorf("code d'erreur: attendu corps_invalide, obtenu %q", code)
	}
}

func TestCreationNomInvalide(t *testing.T) {
	cas := []struct {
		nom   string
		corps string
	}{
		{"vide", `{"nom":"   "}`},
		{"trop long", `{"nom":"` + strings.Repeat("a", 101) + `"}`},
	}
	for _, tc := range cas {
		t.Run(tc.nom, func(t *testing.T) {
			f := &fauxStore{}
			router := NewRouter(depsAvecCode(f))
			rec := postApplication(t, router, "code-invitation-test", tc.corps)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("code HTTP: attendu 400, obtenu %d", rec.Code)
			}
			if code := codeErreur(t, rec); code != "nom_invalide" {
				t.Errorf("code d'erreur: attendu nom_invalide, obtenu %q", code)
			}
		})
	}
}

func TestCreationChampInconnu(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecCode(f))
	rec := postApplication(t, router, "code-invitation-test", `{"nom":"App","inconnu":1}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code HTTP: attendu 400, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "corps_invalide" {
		t.Errorf("code d'erreur: attendu corps_invalide, obtenu %q", code)
	}
}

func TestCreationNomDejaPris(t *testing.T) {
	f := &fauxStore{err: store.ErrNomDejaUtilise}
	router := NewRouter(depsAvecCode(f))
	rec := postApplication(t, router, "code-invitation-test", `{"nom":"App"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("code HTTP: attendu 409, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "nom_deja_utilise" {
		t.Errorf("code d'erreur: attendu nom_deja_utilise, obtenu %q", code)
	}
}

func TestCreationErreurInterne(t *testing.T) {
	detail := "ERREUR_TECHNIQUE_CONFIDENTIELLE_98765"
	f := &fauxStore{err: errors.New(detail)}
	router := NewRouter(depsAvecCode(f))
	rec := postApplication(t, router, "code-invitation-test", `{"nom":"App"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("code HTTP: attendu 500, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "erreur_interne" {
		t.Errorf("code d'erreur: attendu erreur_interne, obtenu %q", code)
	}
	if strings.Contains(rec.Body.String(), detail) {
		t.Error("le détail technique ne doit pas apparaître dans la réponse")
	}
}

// testUUID est un identifiant d'application valide et absent de toute base.
const testUUID = "123e4567-e89b-12d3-a456-426614174000"

func getApplicationAvec(t *testing.T, router http.Handler, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/application", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func getApplication(t *testing.T, router http.Handler, cle string) *httptest.ResponseRecorder {
	t.Helper()
	var authorization string
	if cle != "" {
		authorization = "Bearer " + cle
	}
	return getApplicationAvec(t, router, authorization)
}

func postAdmin(t *testing.T, router http.Handler, id, action, code string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/applications/"+id+"/"+action, nil)
	if code != "" {
		req.Header.Set("X-Admin-Code", code)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestPortierSansEnTete(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecCode(f))

	rec := getApplication(t, router, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code HTTP: attendu 401, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "cle_invalide" {
		t.Errorf("code d'erreur: attendu cle_invalide, obtenu %q", code)
	}
	if f.callsByKey != 0 {
		t.Error("le portier ne doit pas interroger le store sans en-tête Authorization")
	}
}

func TestPortierHeaderBasique(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecCode(f))

	rec := getApplicationAvec(t, router, "Basic xxx")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code HTTP: attendu 401, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "cle_invalide" {
		t.Errorf("code d'erreur: attendu cle_invalide, obtenu %q", code)
	}
	if f.callsByKey != 0 {
		t.Error("le portier ne doit pas interroger le store avec un en-tête mal formé")
	}
}

func TestPortierCleSansPrefixe(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecCode(f))

	rec := getApplicationAvec(t, router, "Bearer aucune-cle")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code HTTP: attendu 401, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "cle_invalide" {
		t.Errorf("code d'erreur: attendu cle_invalide, obtenu %q", code)
	}
	if f.callsByKey != 0 {
		t.Error("le portier ne doit pas interroger le store pour une clé sans préfixe kvk_")
	}
}

func TestPortierCleMalLongueur(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecCode(f))

	rec := getApplicationAvec(t, router, "Bearer kvk_short")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code HTTP: attendu 401, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "cle_invalide" {
		t.Errorf("code d'erreur: attendu cle_invalide, obtenu %q", code)
	}
	if f.callsByKey != 0 {
		t.Error("le portier ne doit pas interroger le store pour une clé de mauvaise longueur")
	}
}

func TestPortierCleInconnue(t *testing.T) {
	cle := "kvk_" + strings.Repeat("a", 64)
	f := &fauxStore{errByKey: store.ErrApplicationIntrouvable}
	router := NewRouter(depsAvecCode(f))

	recAbsent := getApplication(t, router, "")
	recInconnu := getApplication(t, router, cle)
	if recAbsent.Code != http.StatusUnauthorized || recInconnu.Code != http.StatusUnauthorized {
		t.Errorf("code HTTP: attendu 401, obtenus %d et %d", recAbsent.Code, recInconnu.Code)
	}
	if code := codeErreur(t, recInconnu); code != "cle_invalide" {
		t.Errorf("code d'erreur: attendu cle_invalide, obtenu %q", code)
	}
	if recAbsent.Body.String() != recInconnu.Body.String() {
		t.Errorf("les corps doivent être identiques, obtenus %q et %q", recAbsent.Body.String(), recInconnu.Body.String())
	}
}

func TestPortierApplicationDesactivee(t *testing.T) {
	cle := "kvk_" + strings.Repeat("b", 64)
	f := &fauxStore{byKey: store.Application{ID: "app-1", Nom: "App Desactivee", EstActive: false}}
	router := NewRouter(depsAvecCode(f))

	rec := getApplication(t, router, cle)
	if rec.Code != http.StatusForbidden {
		t.Errorf("code HTTP: attendu 403, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "application_desactivee" {
		t.Errorf("code d'erreur: attendu application_desactivee, obtenu %q", code)
	}
}

func TestPortierApplicationActive(t *testing.T) {
	cle := "kvk_" + strings.Repeat("c", 64)
	f := &fauxStore{byKey: store.Application{ID: "app-1", Nom: "Nom Actif", EstActive: true}}
	router := NewRouter(depsAvecCode(f))

	rec := getApplication(t, router, cle)
	if rec.Code != http.StatusOK {
		t.Fatalf("code HTTP: attendu 200, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	var rep struct {
		Nom string `json:"nom"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatalf("réponse JSON invalide: %v", err)
	}
	if rep.Nom != "Nom Actif" {
		t.Errorf("nom: attendu %q, obtenu %q", "Nom Actif", rep.Nom)
	}
	if f.callsByKey != 1 {
		t.Errorf("le store doit être appelé une fois, obtenu %d", f.callsByKey)
	}
	if f.empreinteRecue != apikey.Hash(cle) {
		t.Error("le portier doit transmettre l'empreinte de la clé au store")
	}
	if strings.Contains(f.empreinteRecue, cle) {
		t.Error("le portier ne doit jamais transmettre la clé en clair au store")
	}
}

func TestPortierErreurTechnique(t *testing.T) {
	cle := "kvk_" + strings.Repeat("d", 64)
	f := &fauxStore{errByKey: errors.New("ERREUR_TECHNIQUE_SECRETE_123")}
	router := NewRouter(depsAvecCode(f))

	rec := getApplication(t, router, cle)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("code HTTP: attendu 500, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "erreur_interne" {
		t.Errorf("code d'erreur: attendu erreur_interne, obtenu %q", code)
	}
	if strings.Contains(rec.Body.String(), "ERREUR_TECHNIQUE_SECRETE_123") {
		t.Error("le détail technique ne doit pas apparaître dans la réponse")
	}
}

func TestAdminSansCodeConfigure(t *testing.T) {
	f := &fauxStore{}
	deps := depsAvecAdmin(f, "")
	router := NewRouter(deps)

	rec := postAdmin(t, router, testUUID, "desactiver", "")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("code HTTP: attendu 503, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "administration_desactivee" {
		t.Errorf("code d'erreur: attendu administration_desactivee, obtenu %q", code)
	}
	if f.callsActive != 0 {
		t.Error("le store ne doit pas être appelé quand l'administration est désactivée")
	}
}

func TestAdminSansEnTete(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecAdmin(f, "code-admin-test"))

	rec := postAdmin(t, router, testUUID, "desactiver", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code HTTP: attendu 401, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "code_admin_invalide" {
		t.Errorf("code d'erreur: attendu code_admin_invalide, obtenu %q", code)
	}
	if f.callsActive != 0 {
		t.Error("le store ne doit pas être appelé sans X-Admin-Code")
	}
}

func TestAdminMauvaisCode(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecAdmin(f, "code-admin-test"))

	rec := postAdmin(t, router, testUUID, "desactiver", "mauvais-code")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code HTTP: attendu 401, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "code_admin_invalide" {
		t.Errorf("code d'erreur: attendu code_admin_invalide, obtenu %q", code)
	}
}

func TestAdminInvitationCodeRefuseSurAdmin(t *testing.T) {
	f := &fauxStore{}
	deps := depsAvecCode(f)
	deps.AdminCode = "code-admin-test"
	router := NewRouter(deps)

	rec := postAdmin(t, router, testUUID, "desactiver", deps.InvitationCode)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code HTTP: attendu 401, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "code_admin_invalide" {
		t.Errorf("code d'erreur: attendu code_admin_invalide, obtenu %q", code)
	}
}

func TestAdminCodeAdminRefuseSurInvitation(t *testing.T) {
	f := &fauxStore{}
	deps := depsAvecAdmin(f, "code-admin-test")
	router := NewRouter(deps)

	rec := postApplication(t, router, "code-admin-test", `{"nom":"App"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code HTTP: attendu 401, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	if code := codeErreur(t, rec); code != "code_invitation_invalide" {
		t.Errorf("code d'erreur: attendu code_invitation_invalide, obtenu %q", code)
	}
}

func TestAdminIdentifiantInvalide(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecAdmin(f, "code-admin-test"))

	rec := postAdmin(t, router, "abc", "desactiver", "code-admin-test")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code HTTP: attendu 400, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "identifiant_invalide" {
		t.Errorf("code d'erreur: attendu identifiant_invalide, obtenu %q", code)
	}
	if f.callsActive != 0 {
		t.Error("le store ne doit pas être appelé pour un identifiant invalide")
	}
}

func TestAdminApplicationIntrouvable(t *testing.T) {
	f := &fauxStore{errActive: store.ErrApplicationIntrouvable}
	router := NewRouter(depsAvecAdmin(f, "code-admin-test"))

	rec := postAdmin(t, router, testUUID, "desactiver", "code-admin-test")
	if rec.Code != http.StatusNotFound {
		t.Errorf("code HTTP: attendu 404, obtenu %d", rec.Code)
	}
	if code := codeErreur(t, rec); code != "application_introuvable" {
		t.Errorf("code d'erreur: attendu application_introuvable, obtenu %q", code)
	}
}

func TestAdminDesactiverActiver(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecAdmin(f, "code-admin-test"))

	rec := postAdmin(t, router, testUUID, "desactiver", "code-admin-test")
	if rec.Code != http.StatusOK {
		t.Fatalf("code HTTP: attendu 200, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	var repDesactiver struct {
		ID        string `json:"id_application"`
		EstActive bool   `json:"est_active"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &repDesactiver); err != nil {
		t.Fatalf("réponse JSON invalide: %v", err)
	}
	if repDesactiver.EstActive {
		t.Error("est_active doit être false après désactivation")
	}
	if f.idActiveRecu != testUUID || f.activeRecu {
		t.Errorf("store appelé avec (%q, %v), attendu (%q, false)", f.idActiveRecu, f.activeRecu, testUUID)
	}

	rec = postAdmin(t, router, testUUID, "activer", "code-admin-test")
	if rec.Code != http.StatusOK {
		t.Fatalf("code HTTP: attendu 200, obtenu %d (%s)", rec.Code, rec.Body.String())
	}
	var repActiver struct {
		ID        string `json:"id_application"`
		EstActive bool   `json:"est_active"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &repActiver); err != nil {
		t.Fatalf("réponse JSON invalide: %v", err)
	}
	if !repActiver.EstActive {
		t.Error("est_active doit être true après activation")
	}
	if f.idActiveRecu != testUUID || !f.activeRecu {
		t.Errorf("store appelé avec (%q, %v), attendu (%q, true)", f.idActiveRecu, f.activeRecu, testUUID)
	}
}

func TestAdminDesactiverDeuxFois(t *testing.T) {
	f := &fauxStore{}
	router := NewRouter(depsAvecAdmin(f, "code-admin-test"))

	for i := 0; i < 2; i++ {
		rec := postAdmin(t, router, testUUID, "desactiver", "code-admin-test")
		if rec.Code != http.StatusOK {
			t.Fatalf("appel %d : code HTTP: attendu 200, obtenu %d (%s)", i+1, rec.Code, rec.Body.String())
		}
	}
	if f.callsActive != 2 {
		t.Errorf("le store doit être appelé deux fois, obtenu %d", f.callsActive)
	}
}
