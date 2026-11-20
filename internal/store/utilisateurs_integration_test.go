package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"keyavi-videos/internal/testdb"
)

func TestObtenirOuCreerUtilisateurIntegration(t *testing.T) {
	pool := testdb.Open(t)
	s := NewStore(pool)
	ctx := context.Background()

	app, err := s.CreateApplication(ctx, NewApplication{Nom: "App Utilisateurs", EmpreinteCle: strings.Repeat("a1", 32)})
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}

	u, cree, err := s.ObtenirOuCreerUtilisateur(ctx, app.ID, "u1")
	if err != nil {
		t.Fatalf("ObtenirOuCreerUtilisateur: %v", err)
	}
	if !cree {
		t.Error("le premier appel doit signaler la création")
	}
	if u.IDExterne != "u1" {
		t.Errorf("id externe: attendu %q, obtenu %q", "u1", u.IDExterne)
	}
	if u.Role != RoleLecteur {
		t.Errorf("rôle: attendu %q, obtenu %q", RoleLecteur, u.Role)
	}

	u2, cree2, err := s.ObtenirOuCreerUtilisateur(ctx, app.ID, "u1")
	if err != nil {
		t.Fatalf("ObtenirOuCreerUtilisateur (second appel): %v", err)
	}
	if cree2 {
		t.Error("le second appel ne doit pas signaler de création")
	}
	if u2.IDExterne != "u1" {
		t.Errorf("id externe: attendu %q, obtenu %q", "u1", u2.IDExterne)
	}
}

func TestObtenirOuCreerUtilisateurConcurrent(t *testing.T) {
	pool := testdb.Open(t)
	s := NewStore(pool)
	ctx := context.Background()

	app, err := s.CreateApplication(ctx, NewApplication{Nom: "App Concurrent", EmpreinteCle: strings.Repeat("a2", 32)})
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}

	const n = 2
	crees := make([]bool, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, cree, err := s.ObtenirOuCreerUtilisateur(ctx, app.ID, "u1")
			crees[i] = cree
			errs[i] = err
		}(i)
	}
	wg.Wait()

	nombreCrees := 0
	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("ObtenirOuCreerUtilisateur (appel %d): %v", i, errs[i])
		}
		if crees[i] {
			nombreCrees++
		}
	}
	if nombreCrees != 1 {
		t.Errorf("exactement un appel doit signaler la création, obtenu %d", nombreCrees)
	}
}

func TestDefinirRoleUtilisateurIntegration(t *testing.T) {
	pool := testdb.Open(t)
	s := NewStore(pool)
	ctx := context.Background()

	app, err := s.CreateApplication(ctx, NewApplication{Nom: "App Rôles", EmpreinteCle: strings.Repeat("a3", 32)})
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}

	// Crée un absent.
	u, err := s.DefinirRole(ctx, app.ID, "newbie", RoleEditeur)
	if err != nil {
		t.Fatalf("DefinirRole (création): %v", err)
	}
	if u.IDExterne != "newbie" {
		t.Errorf("id externe: attendu %q, obtenu %q", "newbie", u.IDExterne)
	}
	if u.Role != RoleEditeur {
		t.Errorf("rôle: attendu %q, obtenu %q", RoleEditeur, u.Role)
	}

	// Modifie un existant.
	if _, err := s.DefinirRole(ctx, app.ID, "existing", RoleAdministrateur); err != nil {
		t.Fatalf("DefinirRole (premier) : %v", err)
	}
	u2, err := s.DefinirRole(ctx, app.ID, "existing", RoleLecteur)
	if err != nil {
		t.Fatalf("DefinirRole (modification): %v", err)
	}
	if u2.Role != RoleLecteur {
		t.Errorf("rôle: attendu %q, obtenu %q", RoleLecteur, u2.Role)
	}

	// Vérifie en base.
	var role string
	err = pool.QueryRow(ctx,
		"SELECT role FROM utilisateur_externe WHERE id_application = $1::uuid AND id_externe = $2",
		app.ID, "existing",
	).Scan(&role)
	if err != nil {
		t.Fatalf("lecture en base: %v", err)
	}
	if role != RoleLecteur {
		t.Errorf("rôle en base: attendu %q, obtenu %q", RoleLecteur, role)
	}
}

func TestGetUtilisateurAbsentIntegration(t *testing.T) {
	pool := testdb.Open(t)
	s := NewStore(pool)
	ctx := context.Background()

	app, err := s.CreateApplication(ctx, NewApplication{Nom: "App Introuvable", EmpreinteCle: strings.Repeat("a4", 32)})
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}

	_, err = s.GetUtilisateur(ctx, app.ID, "nope")
	if !errors.Is(err, ErrUtilisateurIntrouvable) {
		t.Errorf("attendu ErrUtilisateurIntrouvable, obtenu %v", err)
	}
}

func TestIsolationUtilisateursEntreApplicationsIntegration(t *testing.T) {
	pool := testdb.Open(t)
	s := NewStore(pool)
	ctx := context.Background()

	appA, err := s.CreateApplication(ctx, NewApplication{Nom: "App A", EmpreinteCle: strings.Repeat("a5", 32)})
	if err != nil {
		t.Fatalf("CreateApplication A: %v", err)
	}
	appB, err := s.CreateApplication(ctx, NewApplication{Nom: "App B", EmpreinteCle: strings.Repeat("b5", 32)})
	if err != nil {
		t.Fatalf("CreateApplication B: %v", err)
	}

	uA, creeA, err := s.ObtenirOuCreerUtilisateur(ctx, appA.ID, "u1")
	if err != nil || !creeA {
		t.Fatalf("ObtenirOuCreerUtilisateur (A): %v", err)
	}
	if uA.Role != RoleLecteur {
		t.Errorf("rôle: attendu %q, obtenu %q", RoleLecteur, uA.Role)
	}
	uB, creeB, err := s.ObtenirOuCreerUtilisateur(ctx, appB.ID, "u1")
	if err != nil || !creeB {
		t.Fatalf("ObtenirOuCreerUtilisateur (B): %v", err)
	}
	if uB.Role != RoleLecteur {
		t.Errorf("rôle: attendu %q, obtenu %q", RoleLecteur, uB.Role)
	}

	// Changer le rôle dans A ne change pas B.
	if _, err := s.DefinirRole(ctx, appA.ID, "u1", RoleAdministrateur); err != nil {
		t.Fatalf("DefinirRole (A): %v", err)
	}
	uB2, err := s.GetUtilisateur(ctx, appB.ID, "u1")
	if err != nil {
		t.Fatalf("GetUtilisateur (B): %v", err)
	}
	if uB2.Role != RoleLecteur {
		t.Errorf("rôle de B doit rester %q, obtenu %q", RoleLecteur, uB2.Role)
	}

	// Supprimer dans A ne touche pas à B.
	if err := s.SupprimerUtilisateur(ctx, appA.ID, "u1", false); err != nil {
		t.Fatalf("SupprimerUtilisateur (A): %v", err)
	}
	uB3, err := s.GetUtilisateur(ctx, appB.ID, "u1")
	if err != nil {
		t.Fatalf("l'utilisateur de B doit survivre : %v", err)
	}
	if uB3.Role != RoleLecteur {
		t.Errorf("rôle de B doit rester %q, obtenu %q", RoleLecteur, uB3.Role)
	}
}

func TestSupprimerUtilisateurSansVideosIntegration(t *testing.T) {
	pool := testdb.Open(t)
	s := NewStore(pool)
	ctx := context.Background()

	app, err := s.CreateApplication(ctx, NewApplication{Nom: "App Suppression 1", EmpreinteCle: strings.Repeat("a6", 32)})
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}

	u, cree, err := s.ObtenirOuCreerUtilisateur(ctx, app.ID, "u1")
	if err != nil || !cree {
		t.Fatalf("ObtenirOuCreerUtilisateur: %v", err)
	}
	if u.IDExterne != "u1" {
		t.Errorf("id externe: attendu %q, obtenu %q", "u1", u.IDExterne)
	}

	// Idempotent : supprimer un absent ne renvoie pas d'erreur.
	if err := s.SupprimerUtilisateur(ctx, app.ID, "absent", false); err != nil {
		t.Errorf("supprimer un absent ne doit pas renvoyer d'erreur, obtenu %v", err)
	}

	// Insère la vidéo de l'utilisateur directement en SQL.
	_, err = pool.Exec(ctx, `
		INSERT INTO video (id_application, id_auteur, titre, cle_idempotence)
		VALUES ($1::uuid,
		        (SELECT id_utilisateur FROM utilisateur_externe WHERE id_application = $1::uuid AND id_externe = $2),
		        'Vidéo test 1', 'idem-video-1')`,
		app.ID, "u1")
	if err != nil {
		t.Fatalf("insertion de la vidéo: %v", err)
	}

	// Suppression avec supprimerVideos=false : l'utilisateur disparaît, la vidéo reste avec id_auteur NULL.
	if err := s.SupprimerUtilisateur(ctx, app.ID, "u1", false); err != nil {
		t.Fatalf("SupprimerUtilisateur: %v", err)
	}
	if _, err := s.GetUtilisateur(ctx, app.ID, "u1"); !errors.Is(err, ErrUtilisateurIntrouvable) {
		t.Errorf("l'utilisateur doit être supprimé, obtenu %v", err)
	}

	var auteur *string
	err = pool.QueryRow(ctx,
		`SELECT id_auteur::text FROM video WHERE id_application = $1::uuid AND cle_idempotence = 'idem-video-1'`,
		app.ID,
	).Scan(&auteur)
	if err != nil {
		t.Fatalf("lecture de la vidéo: %v", err)
	}
	if auteur != nil {
		t.Errorf("l'auteur de la vidéo doit être NULL, obtenu %q", *auteur)
	}
}

func TestSupprimerUtilisateurAvecVideosIntegration(t *testing.T) {
	pool := testdb.Open(t)
	s := NewStore(pool)
	ctx := context.Background()

	app, err := s.CreateApplication(ctx, NewApplication{Nom: "App Suppression 2", EmpreinteCle: strings.Repeat("a7", 32)})
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}

	_, cree, err := s.ObtenirOuCreerUtilisateur(ctx, app.ID, "u2")
	if err != nil || !cree {
		t.Fatalf("ObtenirOuCreerUtilisateur: %v", err)
	}

	// Insère la vidéo de l'utilisateur directement en SQL.
	_, err = pool.Exec(ctx, `
		INSERT INTO video (id_application, id_auteur, titre, cle_idempotence)
		VALUES ($1::uuid,
		        (SELECT id_utilisateur FROM utilisateur_externe WHERE id_application = $1::uuid AND id_externe = $2),
		        'Vidéo test 2', 'idem-video-2')`,
		app.ID, "u2")
	if err != nil {
		t.Fatalf("insertion de la vidéo: %v", err)
	}

	// Suppression avec supprimerVideos=true : l'utilisateur et la vidéo disparaissent.
	if err := s.SupprimerUtilisateur(ctx, app.ID, "u2", true); err != nil {
		t.Fatalf("SupprimerUtilisateur: %v", err)
	}
	if _, err := s.GetUtilisateur(ctx, app.ID, "u2"); !errors.Is(err, ErrUtilisateurIntrouvable) {
		t.Errorf("l'utilisateur doit être supprimé, obtenu %v", err)
	}

	var nb int
	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM video WHERE id_application = $1::uuid AND cle_idempotence = 'idem-video-2'`,
		app.ID,
	).Scan(&nb)
	if err != nil {
		t.Fatalf("comptage des vidéos: %v", err)
	}
	if nb != 0 {
		t.Errorf("la vidéo doit être supprimée, obtenu %d", nb)
	}
}
