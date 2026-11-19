// Package testdb prépare une base de test vide pour les tests d'intégration.
package testdb

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"keyavi-videos/internal/db"

	"github.com/jackc/pgx/v5/pgxpool"
)

// constCleVerrou est la constante entière du verrou PostgreSQL de session
// (pg_advisory_lock) tenant durant toute l'utilisation de la base de test.
const constCleVerrou = 7424001

// racineDepot remonte l'arborescence jusqu'au fichier go.mod.
func racineDepot(t *testing.T) string {
	t.Helper()

	dossier, err := os.Getwd()
	if err != nil {
		t.Fatalf("répertoire courant introuvable: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dossier, "go.mod")); err == nil {
			return dossier
		}
		parent := filepath.Dir(dossier)
		if parent == dossier {
			t.Fatal("fichier go.mod introuvable en remontant depuis le répertoire de test")
		}
		dossier = parent
	}
}

// Open ouvre une base de test vide, applique les migrations et nettoie à la fin.
// Un verrou de session (advisory lock) tient pendant toute la durée de chaque
// test : deux paquets de tests lancés en parallèle sur la même base se
// gêneraient autrement ("la base de test n'est pas vide").
func Open(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL non défini : test d'intégration ignoré")
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connexion à la base de test impossible: %v", err)
	}

	// Sécurité : le nom de la base doit se terminer par "_test".
	var nomBase string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&nomBase); err != nil {
		pool.Close()
		t.Fatalf("lecture du nom de la base impossible: %v", err)
	}
	if !strings.HasSuffix(nomBase, "_test") {
		pool.Close()
		t.Fatal("refus d'utiliser une base qui n'est pas une base de test (nom ne finissant pas par _test)")
	}

	// Verrou de session (advisory lock) : pris sur une connexion dédiée ; il est
	// relâché pendant le nettoyage, sur la même connexion. Si une étape échoue
	// avant la fin de Open(), on libère la connexion et on ferme le pool.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		pool.Close()
		t.Fatalf("acquisition de la connexion de verrou impossible: %v", err)
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", constCleVerrou); err != nil {
		conn.Release()
		pool.Close()
		t.Fatalf("verrou de session impossible: %v", err)
	}

	// Sécurité : le schéma public doit être vide avant toute migration.
	// La table schema_migrations est ignorée : elle appartient à golang-migrate
	// et subsiste après un Down(), elle ne contient aucune donnée applicative.
	var nbTables int
	err = pool.QueryRow(ctx,
		"SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE' AND table_name <> 'schema_migrations'",
	).Scan(&nbTables)
	if err != nil {
		conn.Release()
		pool.Close()
		t.Fatalf("comptage des tables impossible: %v", err)
	}
	if nbTables != 0 {
		conn.Release()
		pool.Close()
		t.Fatal("la base de test n'est pas vide")
	}

	migrations := filepath.Join(racineDepot(t), "migrations")
	if err := db.MigrerHaut(url, migrations); err != nil {
		conn.Release()
		pool.Close()
		t.Fatalf("migrations impossibles: %v", err)
	}

	t.Cleanup(func() {
		if err := db.MigrerBas(url, migrations); err != nil {
			t.Errorf("nettoyage des migrations impossible: %v", err)
		}
		if _, err := conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", constCleVerrou); err != nil {
			t.Errorf("relâchement du verrou de session impossible: %v", err)
		}
		conn.Release()
		pool.Close()
	})

	return pool
}
