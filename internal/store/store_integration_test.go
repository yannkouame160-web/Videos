package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"keyavi-videos/internal/apikey"
	"keyavi-videos/internal/testdb"
)

func TestCreateApplicationIntegration(t *testing.T) {
	pool := testdb.Open(t)
	s := NewStore(pool)
	ctx := context.Background()

	app, err := s.CreateApplication(ctx, NewApplication{
		Nom:                     "App Integration",
		EmpreinteCle:            "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		PlafondStockageOctets:   2048,
		PlafondNbVideos:         42,
		LimiteRequetesParMinute: 7,
	})
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}
	if app.ID == "" {
		t.Error("l'ID renvoyé ne doit pas être vide")
	}
	if app.Nom != "App Integration" {
		t.Errorf("nom: attendu %q, obtenu %q", "App Integration", app.Nom)
	}
	if !app.EstActive {
		t.Error("est_active doit être true")
	}

	var actif bool
	var stockage int64
	var videos, requetes int
	err = pool.QueryRow(ctx, `
		SELECT est_active, plafond_stockage_octets, plafond_nb_videos, limite_requetes_par_minute
		FROM application WHERE id_application = $1::uuid`, app.ID,
	).Scan(&actif, &stockage, &videos, &requetes)
	if err != nil {
		t.Fatalf("lecture de l'application: %v", err)
	}
	if !actif || stockage != 2048 || videos != 42 || requetes != 7 {
		t.Errorf("valeurs en base incorrectes: actif=%v stockage=%d vidéos=%d requêtes=%d",
			actif, stockage, videos, requetes)
	}
}

func TestCreateApplicationEmpreinteSeulement(t *testing.T) {
	pool := testdb.Open(t)
	s := NewStore(pool)
	ctx := context.Background()

	cle, empreinte, err := apikey.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	app, err := s.CreateApplication(ctx, NewApplication{
		Nom:          "App Empreinte",
		EmpreinteCle: empreinte,
	})
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}

	var empreinteStockee, ligne string
	err = pool.QueryRow(ctx, `
		SELECT empreinte_cle, to_jsonb(a)::text
		FROM application a WHERE id_application = $1::uuid`, app.ID,
	).Scan(&empreinteStockee, &ligne)
	if err != nil {
		t.Fatalf("lecture de l'application: %v", err)
	}
	if empreinteStockee != empreinte {
		t.Errorf("empreinte stockée incorrecte: attendu %q, obtenu %q", empreinte, empreinteStockee)
	}
	if strings.Contains(ligne, cle) {
		t.Error("la clé en clair ne doit apparaître dans aucune colonne de la ligne")
	}
}

func TestCreateApplicationNomDejaUtilise(t *testing.T) {
	pool := testdb.Open(t)
	s := NewStore(pool)
	ctx := context.Background()

	donnees := NewApplication{
		Nom:          "App Doublon Nom",
		EmpreinteCle: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	if _, err := s.CreateApplication(ctx, donnees); err != nil {
		t.Fatalf("premier CreateApplication: %v", err)
	}

	donnees.EmpreinteCle = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	_, err := s.CreateApplication(ctx, donnees)
	if !errors.Is(err, ErrNomDejaUtilise) {
		t.Errorf("attendu ErrNomDejaUtilise, obtenu %v", err)
	}
}

func TestCreateApplicationEmpreinteDejaUtilisee(t *testing.T) {
	pool := testdb.Open(t)
	s := NewStore(pool)
	ctx := context.Background()

	empreinte := "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	if _, err := s.CreateApplication(ctx, NewApplication{
		Nom: "App Un", EmpreinteCle: empreinte,
	}); err != nil {
		t.Fatalf("premier CreateApplication: %v", err)
	}

	_, err := s.CreateApplication(ctx, NewApplication{
		Nom: "App Deux", EmpreinteCle: empreinte,
	})
	if err == nil {
		t.Error("une deuxième application avec la même empreinte devait être refusée")
	}
}
