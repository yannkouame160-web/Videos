#!/usr/bin/env bash
set -euo pipefail

# On se place à la racine du dépôt, quel que soit le répertoire d'appel.
cd "$(dirname "$0")/.."

# Chargement des variables du .env. La valeur de DATABASE_URL n'est jamais affichée.
set -a
. ./.env
set +a

PSQL_OPTS=(-v ON_ERROR_STOP=1)

fail() {
  echo "ECHEC : $1" >&2
  exit 1
}

# Sécurité : la base de test doit être vide avant toute modification.
nb_tables=$(psql "$DATABASE_URL" "${PSQL_OPTS[@]}" -tAc \
  "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE' AND table_name <> 'schema_migrations'") \
  || fail "impossible de compter les tables de la base de test"
if [ "$nb_tables" -ne 0 ]; then
  fail "la base de test n'est pas vide ($nb_tables table(s) dans le schéma public). Rien n'a été modifié."
fi

# PostgreSQL 15 ou plus est requis (ON DELETE SET NULL avec liste de colonnes).
version=$(psql "$DATABASE_URL" "${PSQL_OPTS[@]}" -tAc "SHOW server_version_num") \
  || fail "impossible de lire la version du serveur"
if [ "$version" -lt 150000 ]; then
  fail "PostgreSQL 15 ou plus requis (version détectée : $version)."
fi

# Application du schéma puis exécution des tests.
psql "$DATABASE_URL" "${PSQL_OPTS[@]}" -q -f migrations/000001_schema_initial.up.sql \
  || fail "échec de l'application de la migration up"
psql "$DATABASE_URL" "${PSQL_OPTS[@]}" -f scripts/test_schema.sql \
  || fail "échec des tests SQL"

# Retour arrière et vérification que la base est de nouveau propre.
psql "$DATABASE_URL" "${PSQL_OPTS[@]}" -q -f migrations/000001_schema_initial.down.sql \
  || fail "échec de l'application de la migration down"

verifier_base_propre() {
  local tables types
  tables=$(psql "$DATABASE_URL" "${PSQL_OPTS[@]}" -tAc \
    "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE' AND table_name <> 'schema_migrations'") \
    || fail "impossible de revérifier les tables"
  types=$(psql "$DATABASE_URL" "${PSQL_OPTS[@]}" -tAc \
    "SELECT count(*) FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace WHERE n.nspname = 'public' AND t.typtype IN ('e', 'c', 'd', 'r') AND t.typname <> 'schema_migrations'") \
    || fail "impossible de revérifier les types personnalisés"
  if [ "$tables" -ne 0 ] || [ "$types" -ne 0 ]; then
    fail "la base n'est pas propre après down : $tables table(s), $types type(s) personnalisé(s) restant(s)"
  fi
}

verifier_base_propre

# Preuve que la séquence up / down / up fonctionne, puis on laisse la base propre.
psql "$DATABASE_URL" "${PSQL_OPTS[@]}" -q -f migrations/000001_schema_initial.up.sql \
  || fail "échec de la deuxième application de la migration up"
psql "$DATABASE_URL" "${PSQL_OPTS[@]}" -q -f migrations/000001_schema_initial.down.sql \
  || fail "échec de la deuxième application de la migration down"
verifier_base_propre

echo "TOUS LES TESTS ONT RÉUSSI"
