-- Migration 1 : schéma initial du service central de vidéos Keyavi.
-- Source : "Script PostgreSQL service central de vidéos" (parties 1 à 4).
-- Nécessite PostgreSQL 15 ou plus (ON DELETE SET NULL avec liste de colonnes).

-- ============================================================
-- Partie 1 : types et tables principales
-- ============================================================

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE role_utilisateur AS ENUM ('lecteur', 'editeur', 'administrateur');

CREATE TYPE etat_video AS ENUM (
  'attente_envoi', 'envoyee', 'en_traitement', 'prete',
  'attente_validation', 'refusee', 'supprimee'
);

CREATE TYPE etat_tache AS ENUM ('en_attente', 'en_cours', 'reussie', 'echec_definitif');

CREATE TYPE type_signal AS ENUM ('vue_complete', 'vue_rapide', 'aime', 'partage');

CREATE TYPE qualite_video AS ENUM ('240p', '480p', '720p');

CREATE TABLE application (
  id_application              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  nom                         VARCHAR(100) NOT NULL UNIQUE,
  empreinte_cle               VARCHAR(64)  NOT NULL,
  est_active                  BOOLEAN      NOT NULL DEFAULT TRUE,
  moderation_activee          BOOLEAN      NOT NULL DEFAULT FALSE,
  creation_auto_utilisateur   BOOLEAN      NOT NULL DEFAULT TRUE,
  delai_revue_secondes        INTEGER,
  plafond_stockage_octets     BIGINT,
  plafond_nb_videos           INTEGER,
  limite_requetes_par_minute  INTEGER,
  date_creation               TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE utilisateur_externe (
  id_utilisateur            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  id_application            UUID NOT NULL REFERENCES application(id_application) ON DELETE CASCADE,
  id_externe                VARCHAR(128) NOT NULL,
  role                      role_utilisateur NOT NULL DEFAULT 'lecteur',
  date_premiere_apparition  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (id_application, id_externe),
  UNIQUE (id_application, id_utilisateur)
);

CREATE TABLE video (
  id_video          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  id_application    UUID NOT NULL,
  id_auteur         UUID,
  titre             VARCHAR(200) NOT NULL,
  description       TEXT,
  duree_secondes    INTEGER,
  etat              etat_video NOT NULL DEFAULT 'attente_envoi',
  motif_decision    TEXT,
  date_decision     TIMESTAMPTZ,
  cle_idempotence   VARCHAR(64) NOT NULL,
  chemin_original   TEXT,
  chemin_apercu     TEXT,
  date_creation     TIMESTAMPTZ NOT NULL DEFAULT now(),
  date_publication  TIMESTAMPTZ,
  UNIQUE (id_application, cle_idempotence),
  UNIQUE (id_application, id_video),
  FOREIGN KEY (id_application, id_auteur)
    REFERENCES utilisateur_externe (id_application, id_utilisateur)
    ON DELETE SET NULL (id_auteur)
);

-- ============================================================
-- Partie 2 : tables liées aux vidéos, aux signaux et aux compteurs
-- ============================================================

CREATE TABLE fichier_video (
  id_fichier      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  id_video        UUID NOT NULL REFERENCES video(id_video) ON DELETE CASCADE,
  qualite         qualite_video NOT NULL,
  chemin          TEXT NOT NULL,
  taille_octets   BIGINT NOT NULL CHECK (taille_octets >= 0),
  format          VARCHAR(20) NOT NULL,
  UNIQUE (id_video, qualite)
);

CREATE TABLE etiquette (
  id_etiquette    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  id_application  UUID NOT NULL REFERENCES application(id_application) ON DELETE CASCADE,
  nom             VARCHAR(50) NOT NULL,
  UNIQUE (id_application, nom),
  UNIQUE (id_application, id_etiquette)
);

CREATE TABLE video_etiquette (
  id_video        UUID NOT NULL,
  id_etiquette    UUID NOT NULL,
  id_application  UUID NOT NULL,
  PRIMARY KEY (id_video, id_etiquette),
  FOREIGN KEY (id_application, id_video)
    REFERENCES video (id_application, id_video) ON DELETE CASCADE,
  FOREIGN KEY (id_application, id_etiquette)
    REFERENCES etiquette (id_application, id_etiquette) ON DELETE CASCADE
);

CREATE TABLE tache_traitement (
  id_tache                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  id_video                UUID NOT NULL REFERENCES video(id_video) ON DELETE CASCADE,
  etat                    etat_tache NOT NULL DEFAULT 'en_attente',
  nb_tentatives           INTEGER NOT NULL DEFAULT 0 CHECK (nb_tentatives >= 0),
  dernier_message_erreur  TEXT,
  date_debut              TIMESTAMPTZ,
  date_fin                TIMESTAMPTZ
);

CREATE TABLE signal (
  id_signal                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  id_application           UUID NOT NULL,
  id_utilisateur           UUID NOT NULL,
  id_video                 UUID NOT NULL,
  type                     type_signal NOT NULL,
  duree_regardee_secondes  INTEGER CHECK (duree_regardee_secondes >= 0),
  date_signal              TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (id_application, id_utilisateur)
    REFERENCES utilisateur_externe (id_application, id_utilisateur) ON DELETE CASCADE,
  FOREIGN KEY (id_application, id_video)
    REFERENCES video (id_application, id_video) ON DELETE CASCADE
);

CREATE TABLE vue_utilisateur (
  id_utilisateur     UUID NOT NULL,
  id_video           UUID NOT NULL,
  id_application     UUID NOT NULL,
  date_derniere_vue  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (id_utilisateur, id_video),
  FOREIGN KEY (id_application, id_utilisateur)
    REFERENCES utilisateur_externe (id_application, id_utilisateur) ON DELETE CASCADE,
  FOREIGN KEY (id_application, id_video)
    REFERENCES video (id_application, id_video) ON DELETE CASCADE
);

CREATE TABLE compteur_video (
  id_video     UUID PRIMARY KEY REFERENCES video(id_video) ON DELETE CASCADE,
  nb_vues      BIGINT NOT NULL DEFAULT 0,
  nb_aimes     BIGINT NOT NULL DEFAULT 0,
  nb_partages  BIGINT NOT NULL DEFAULT 0
);

-- ============================================================
-- Partie 3 : index
-- ============================================================

CREATE INDEX idx_video_fil ON video (id_application, etat, date_publication DESC);
CREATE INDEX idx_video_auteur ON video (id_auteur);
CREATE INDEX idx_fichier_video ON fichier_video (id_video);
CREATE INDEX idx_video_etiquette_etiquette ON video_etiquette (id_etiquette);
CREATE INDEX idx_tache_file ON tache_traitement (etat, date_debut);
CREATE INDEX idx_tache_video ON tache_traitement (id_video);
CREATE INDEX idx_signal_utilisateur ON signal (id_utilisateur, date_signal DESC);
CREATE INDEX idx_signal_video ON signal (id_video, type);
CREATE INDEX idx_vue_video ON vue_utilisateur (id_video);

-- AJOUT du développeur (absent du script d'origine) :
-- à chaque appel, l'API retrouve l'application à partir de l'empreinte de sa clé.
-- Cet index unique rend cette recherche rapide et interdit deux applications
-- ayant la même empreinte.
CREATE UNIQUE INDEX uq_application_empreinte_cle ON application (empreinte_cle);

-- ============================================================
-- Partie 4 : compteur automatique, unicité du « aimé », suppression d'un utilisateur
-- ============================================================

CREATE FUNCTION creer_compteur_video() RETURNS trigger AS $$
BEGIN
  INSERT INTO compteur_video (id_video) VALUES (NEW.id_video);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_creer_compteur_video
  AFTER INSERT ON video
  FOR EACH ROW EXECUTE FUNCTION creer_compteur_video();

CREATE UNIQUE INDEX uq_signal_aime
  ON signal (id_utilisateur, id_video)
  WHERE type = 'aime';

CREATE FUNCTION supprimer_utilisateur(
  p_id_application UUID,
  p_id_externe     VARCHAR,
  p_supprimer_videos BOOLEAN DEFAULT FALSE
) RETURNS VOID AS $$
DECLARE
  v_id_utilisateur UUID;
BEGIN
  SELECT id_utilisateur INTO v_id_utilisateur
  FROM utilisateur_externe
  WHERE id_application = p_id_application AND id_externe = p_id_externe;

  IF v_id_utilisateur IS NULL THEN
    RETURN;
  END IF;

  IF p_supprimer_videos THEN
    DELETE FROM video
    WHERE id_application = p_id_application AND id_auteur = v_id_utilisateur;
  END IF;

  DELETE FROM utilisateur_externe WHERE id_utilisateur = v_id_utilisateur;
END;
$$ LANGUAGE plpgsql;
