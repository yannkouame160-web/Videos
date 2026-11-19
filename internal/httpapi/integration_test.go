package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"keyavi-videos/internal/store"
	"keyavi-videos/internal/testdb"
)

func TestCreationApplicationAvecVraieBase(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	deps := Dependencies{
		Store:                    store.NewStore(pool),
		InvitationCode:           "code-invitation-test",
		DefaultStorageQuotaBytes: 1234,
		DefaultMaxVideos:         56,
		DefaultRequestsPerMinute: 78,
	}
	router := NewRouter(deps)

	rec := postApplication(t, router, "code-invitation-test", `{"nom":"App BoutEnBout"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code HTTP: attendu 201, obtenu %d (%s)", rec.Code, rec.Body.String())
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

	var stockage int64
	var videos, requetes int
	err := pool.QueryRow(ctx, `
		SELECT plafond_stockage_octets, plafond_nb_videos, limite_requetes_par_minute
		FROM application WHERE id_application = $1::uuid`, rep.ID,
	).Scan(&stockage, &videos, &requetes)
	if err != nil {
		t.Fatalf("l'application n'existe pas en base: %v", err)
	}
	if stockage != 1234 || videos != 56 || requetes != 78 {
		t.Errorf("plafonds en base incorrects: %d, %d, %d", stockage, videos, requetes)
	}

	rec2 := postApplication(t, router, "code-invitation-test", `{"nom":"App BoutEnBout"}`)
	if rec2.Code != http.StatusConflict {
		t.Errorf("code HTTP du doublon: attendu 409, obtenu %d (%s)", rec2.Code, rec2.Body.String())
	}
	if code := codeErreur(t, rec2); code != "nom_deja_utilise" {
		t.Errorf("code d'erreur: attendu nom_deja_utilise, obtenu %q", code)
	}
}
