-- Tests automatiques du schéma initial Keyavi (PostgreSQL 15+).
-- Tout se déroule dans une seule transaction, annulée à la fin : aucune donnée ne reste.

BEGIN;

-- ------------------------------------------------------------------
-- Données de test : deux applications A et B, un utilisateur et une vidéo dans chacune.
-- ------------------------------------------------------------------
INSERT INTO application (nom, empreinte_cle) VALUES ('App A', 'empreinte-A');
INSERT INTO application (nom, empreinte_cle) VALUES ('App B', 'empreinte-B');

INSERT INTO utilisateur_externe (id_application, id_externe)
SELECT id_application, 'userA' FROM application WHERE nom = 'App A';
INSERT INTO utilisateur_externe (id_application, id_externe)
SELECT id_application, 'userB' FROM application WHERE nom = 'App B';

INSERT INTO video (id_application, id_auteur, titre, cle_idempotence)
SELECT a.id_application, u.id_utilisateur, 'Video A', 'cle-video-A'
  FROM application a
  JOIN utilisateur_externe u ON u.id_application = a.id_application
 WHERE a.nom = 'App A' AND u.id_externe = 'userA';

INSERT INTO video (id_application, id_auteur, titre, cle_idempotence)
SELECT a.id_application, u.id_utilisateur, 'Video B', 'cle-video-B'
  FROM application a
  JOIN utilisateur_externe u ON u.id_application = a.id_application
 WHERE a.nom = 'App B' AND u.id_externe = 'userB';

-- Une étiquette dans B, nécessaire au test d'isolation T3.
INSERT INTO etiquette (id_application, nom)
SELECT id_application, 'Etiquette B' FROM application WHERE nom = 'App B';

-- Petit contexte partagé par tous les tests.
CREATE TEMP TABLE test_ctx AS
SELECT
  (SELECT id_application FROM application WHERE nom = 'App A') AS id_app_a,
  (SELECT id_application FROM application WHERE nom = 'App B') AS id_app_b,
  (SELECT id_utilisateur FROM utilisateur_externe WHERE id_externe = 'userA') AS id_user_a,
  (SELECT id_utilisateur FROM utilisateur_externe WHERE id_externe = 'userB') AS id_user_b,
  (SELECT id_video FROM video WHERE cle_idempotence = 'cle-video-A') AS id_video_a,
  (SELECT id_video FROM video WHERE cle_idempotence = 'cle-video-B') AS id_video_b,
  (SELECT id_etiquette FROM etiquette WHERE nom = 'Etiquette B') AS id_etiquette_b;

-- ------------------------------------------------------------------
-- T1 : un signal de l'utilisateur A sur une vidéo de B doit être refusé.
-- ------------------------------------------------------------------
DO $$
DECLARE
  v_refuse BOOLEAN := FALSE;
BEGIN
  BEGIN
    INSERT INTO signal (id_application, id_utilisateur, id_video, type)
    SELECT c.id_app_a, c.id_user_a, c.id_video_b, 'vue_complete' FROM test_ctx c;
  EXCEPTION WHEN foreign_key_violation THEN
    v_refuse := TRUE;
  END;

  IF v_refuse THEN
    RAISE NOTICE 'OK - T1 signal A sur vidéo B refusé';
  ELSE
    RAISE EXCEPTION 'ECHEC - T1 signal A sur vidéo B non refusé';
  END IF;
END;
$$;

-- ------------------------------------------------------------------
-- T2 : une vue_utilisateur croisée A / B doit être refusée.
-- ------------------------------------------------------------------
DO $$
DECLARE
  v_refuse BOOLEAN := FALSE;
BEGIN
  BEGIN
    INSERT INTO vue_utilisateur (id_utilisateur, id_video, id_application)
    SELECT c.id_user_a, c.id_video_b, c.id_app_a FROM test_ctx c;
  EXCEPTION WHEN foreign_key_violation THEN
    v_refuse := TRUE;
  END;

  IF v_refuse THEN
    RAISE NOTICE 'OK - T2 vue_utilisateur croisée A/B refusée';
  ELSE
    RAISE EXCEPTION 'ECHEC - T2 vue_utilisateur croisée A/B non refusée';
  END IF;
END;
$$;

-- ------------------------------------------------------------------
-- T3 : lier une vidéo de A à une étiquette de B doit être refusé.
-- ------------------------------------------------------------------
DO $$
DECLARE
  v_refuse BOOLEAN := FALSE;
BEGIN
  BEGIN
    INSERT INTO video_etiquette (id_video, id_etiquette, id_application)
    SELECT c.id_video_a, c.id_etiquette_b, c.id_app_a FROM test_ctx c;
  EXCEPTION WHEN foreign_key_violation THEN
    v_refuse := TRUE;
  END;

  IF v_refuse THEN
    RAISE NOTICE 'OK - T3 video_etiquette vidéo A / étiquette B refusée';
  ELSE
    RAISE EXCEPTION 'ECHEC - T3 video_etiquette vidéo A / étiquette B non refusée';
  END IF;
END;
$$;

-- ------------------------------------------------------------------
-- T4 : une vidéo de A avec un auteur de B doit être refusée.
-- ------------------------------------------------------------------
DO $$
DECLARE
  v_refuse BOOLEAN := FALSE;
BEGIN
  BEGIN
    INSERT INTO video (id_application, id_auteur, titre, cle_idempotence)
    SELECT c.id_app_a, c.id_user_b, 'Video croisee', 'cle-croisee' FROM test_ctx c;
  EXCEPTION WHEN foreign_key_violation THEN
    v_refuse := TRUE;
  END;

  IF v_refuse THEN
    RAISE NOTICE 'OK - T4 vidéo A avec auteur B refusée';
  ELSE
    RAISE EXCEPTION 'ECHEC - T4 vidéo A avec auteur B non refusée';
  END IF;
END;
$$;

-- ------------------------------------------------------------------
-- T5 : un seul "aime" par utilisateur et vidéo ; "vue_complete" multiples OK.
-- ------------------------------------------------------------------
DO $$
DECLARE
  v_refuse BOOLEAN := FALSE;
  nb_aime INTEGER;
  nb_vues INTEGER;
BEGIN
  INSERT INTO signal (id_application, id_utilisateur, id_video, type)
  SELECT c.id_app_a, c.id_user_a, c.id_video_a, 'aime' FROM test_ctx c;

  BEGIN
    INSERT INTO signal (id_application, id_utilisateur, id_video, type)
    SELECT c.id_app_a, c.id_user_a, c.id_video_a, 'aime' FROM test_ctx c;
  EXCEPTION WHEN unique_violation THEN
    v_refuse := TRUE;
  END;

  IF NOT v_refuse THEN
    RAISE EXCEPTION 'ECHEC - T5 deuxième aime non refusé';
  END IF;

  INSERT INTO signal (id_application, id_utilisateur, id_video, type)
  SELECT c.id_app_a, c.id_user_a, c.id_video_a, 'vue_complete' FROM test_ctx c;
  INSERT INTO signal (id_application, id_utilisateur, id_video, type)
  SELECT c.id_app_a, c.id_user_a, c.id_video_a, 'vue_complete' FROM test_ctx c;

  SELECT count(*) INTO nb_aime
    FROM signal s, test_ctx c
   WHERE s.id_application = c.id_app_a
     AND s.id_utilisateur = c.id_user_a
     AND s.id_video = c.id_video_a
     AND s.type = 'aime';
  SELECT count(*) INTO nb_vues
    FROM signal s, test_ctx c
   WHERE s.id_application = c.id_app_a
     AND s.id_utilisateur = c.id_user_a
     AND s.id_video = c.id_video_a
     AND s.type = 'vue_complete';

  IF nb_aime <> 1 OR nb_vues <> 2 THEN
    RAISE EXCEPTION 'ECHEC - T5 comptages aime=% vues_complete=%', nb_aime, nb_vues;
  END IF;

  RAISE NOTICE 'OK - T5 unicité du aime et vues complètes multiples acceptées';
END;
$$;

-- ------------------------------------------------------------------
-- T6 : le déclencheur crée automatiquement un compteur_video à chaque vidéo.
-- ------------------------------------------------------------------
DO $$
DECLARE
  nb INTEGER;
  n1 INTEGER;
  n2 INTEGER;
  n3 INTEGER;
BEGIN
  SELECT count(*) INTO nb FROM compteur_video cv, test_ctx c WHERE cv.id_video = c.id_video_a;
  IF nb <> 1 THEN
    RAISE EXCEPTION 'ECHEC - T6 compteur de la vidéo A absent';
  END IF;
  SELECT count(*) INTO nb FROM compteur_video cv, test_ctx c WHERE cv.id_video = c.id_video_b;
  IF nb <> 1 THEN
    RAISE EXCEPTION 'ECHEC - T6 compteur de la vidéo B absent';
  END IF;
  SELECT cv.nb_vues, cv.nb_aimes, cv.nb_partages INTO n1, n2, n3
    FROM compteur_video cv, test_ctx c
   WHERE cv.id_video = c.id_video_a;
  IF n1 <> 0 OR n2 <> 0 OR n3 <> 0 THEN
    RAISE EXCEPTION 'ECHEC - T6 compteur non initialisé à zéro';
  END IF;

  RAISE NOTICE 'OK - T6 déclencheur compteur_video';
END;
$$;

-- ------------------------------------------------------------------
-- T7 : idempotence par (application, cle_idempotence).
-- ------------------------------------------------------------------
DO $$
DECLARE
  v_refuse BOOLEAN := FALSE;
  nb INTEGER;
BEGIN
  BEGIN
    INSERT INTO video (id_application, id_auteur, titre, cle_idempotence)
    SELECT c.id_app_a, c.id_user_a, 'Doublon A', 'cle-video-A' FROM test_ctx c;
  EXCEPTION WHEN unique_violation THEN
    v_refuse := TRUE;
  END;

  IF NOT v_refuse THEN
    RAISE EXCEPTION 'ECHEC - T7 doublon de clé dans la même application non refusé';
  END IF;

  -- La même clé dans une autre application est acceptée.
  INSERT INTO video (id_application, id_auteur, titre, cle_idempotence)
  SELECT c.id_app_b, c.id_user_b, 'Cle A dans B', 'cle-video-A' FROM test_ctx c;

  SELECT count(*) INTO nb
    FROM video v, test_ctx c
   WHERE v.id_application = c.id_app_b AND v.cle_idempotence = 'cle-video-A';
  IF nb <> 1 THEN
    RAISE EXCEPTION 'ECHEC - T7 même clé dans une autre application non acceptée';
  END IF;

  RAISE NOTICE 'OK - T7 idempotence par application';
END;
$$;

-- ------------------------------------------------------------------
-- T8 : empreinte de clé et nom d'application uniques.
-- ------------------------------------------------------------------
DO $$
DECLARE
  v_empreinte BOOLEAN := FALSE;
  v_nom BOOLEAN := FALSE;
BEGIN
  BEGIN
    INSERT INTO application (nom, empreinte_cle) VALUES ('App C', 'empreinte-A');
  EXCEPTION WHEN unique_violation THEN
    v_empreinte := TRUE;
  END;

  BEGIN
    INSERT INTO application (nom, empreinte_cle) VALUES ('App A', 'empreinte-C');
  EXCEPTION WHEN unique_violation THEN
    v_nom := TRUE;
  END;

  IF NOT v_empreinte THEN
    RAISE EXCEPTION 'ECHEC - T8 deuxième empreinte_cle non refusée';
  END IF;
  IF NOT v_nom THEN
    RAISE EXCEPTION 'ECHEC - T8 deuxième nom non refusé';
  END IF;

  RAISE NOTICE 'OK - T8 unicité de empreinte_cle et de nom';
END;
$$;

-- ------------------------------------------------------------------
-- T9 : supprimer_utilisateur(app, id_externe) avec le choix par défaut.
-- ------------------------------------------------------------------
DO $$
DECLARE
  v_app UUID;
  v_user UUID;
  v_video UUID;
  v_auteur UUID;
  nb INTEGER;
BEGIN
  INSERT INTO application (nom, empreinte_cle)
  VALUES ('App T9', 'empreinte-T9') RETURNING id_application INTO v_app;
  INSERT INTO utilisateur_externe (id_application, id_externe)
  VALUES (v_app, 'u9') RETURNING id_utilisateur INTO v_user;
  INSERT INTO video (id_application, id_auteur, titre, cle_idempotence)
  VALUES (v_app, v_user, 'V9', 'cle-T9') RETURNING id_video INTO v_video;
  INSERT INTO signal (id_application, id_utilisateur, id_video, type)
  VALUES (v_app, v_user, v_video, 'vue_complete');
  INSERT INTO vue_utilisateur (id_utilisateur, id_video, id_application)
  VALUES (v_user, v_video, v_app);

  PERFORM supprimer_utilisateur(v_app, 'u9');

  SELECT count(*) INTO nb FROM utilisateur_externe WHERE id_utilisateur = v_user;
  IF nb <> 0 THEN RAISE EXCEPTION 'ECHEC - T9 utilisateur encore présent'; END IF;
  SELECT count(*) INTO nb FROM signal WHERE id_utilisateur = v_user;
  IF nb <> 0 THEN RAISE EXCEPTION 'ECHEC - T9 signaux encore présents'; END IF;
  SELECT count(*) INTO nb FROM vue_utilisateur WHERE id_utilisateur = v_user;
  IF nb <> 0 THEN RAISE EXCEPTION 'ECHEC - T9 vue_utilisateur encore présentes'; END IF;
  SELECT count(*) INTO nb FROM video WHERE id_video = v_video;
  IF nb <> 1 THEN RAISE EXCEPTION 'ECHEC - T9 vidéo supprimée'; END IF;
  SELECT id_auteur INTO v_auteur FROM video WHERE id_video = v_video;
  IF v_auteur IS NOT NULL THEN RAISE EXCEPTION 'ECHEC - T9 id_auteur non mis à NULL'; END IF;
  SELECT count(*) INTO nb FROM compteur_video WHERE id_video = v_video;
  IF nb <> 1 THEN RAISE EXCEPTION 'ECHEC - T9 compteur_video supprimé'; END IF;

  RAISE NOTICE 'OK - T9 suppression utilisateur par défaut';
END;
$$;

-- ------------------------------------------------------------------
-- T10 : supprimer_utilisateur(app, id_externe, TRUE) supprime aussi les vidéos.
-- ------------------------------------------------------------------
DO $$
DECLARE
  v_app UUID;
  v_user UUID;
  v_video UUID;
  nb INTEGER;
BEGIN
  INSERT INTO application (nom, empreinte_cle)
  VALUES ('App T10', 'empreinte-T10') RETURNING id_application INTO v_app;
  INSERT INTO utilisateur_externe (id_application, id_externe)
  VALUES (v_app, 'u10') RETURNING id_utilisateur INTO v_user;
  INSERT INTO video (id_application, id_auteur, titre, cle_idempotence)
  VALUES (v_app, v_user, 'V10', 'cle-T10') RETURNING id_video INTO v_video;
  INSERT INTO fichier_video (id_video, qualite, chemin, taille_octets, format)
  VALUES (v_video, '720p', '/v10.mp4', 100, 'mp4');
  INSERT INTO tache_traitement (id_video) VALUES (v_video);
  INSERT INTO signal (id_application, id_utilisateur, id_video, type)
  VALUES (v_app, v_user, v_video, 'aime');
  INSERT INTO vue_utilisateur (id_utilisateur, id_video, id_application)
  VALUES (v_user, v_video, v_app);

  PERFORM supprimer_utilisateur(v_app, 'u10', TRUE);

  SELECT count(*) INTO nb FROM utilisateur_externe WHERE id_utilisateur = v_user;
  IF nb <> 0 THEN RAISE EXCEPTION 'ECHEC - T10 utilisateur encore présent'; END IF;
  SELECT count(*) INTO nb FROM video WHERE id_video = v_video;
  IF nb <> 0 THEN RAISE EXCEPTION 'ECHEC - T10 vidéo encore présente'; END IF;
  SELECT count(*) INTO nb FROM fichier_video WHERE id_video = v_video;
  IF nb <> 0 THEN RAISE EXCEPTION 'ECHEC - T10 fichier_video encore présent'; END IF;
  SELECT count(*) INTO nb FROM tache_traitement WHERE id_video = v_video;
  IF nb <> 0 THEN RAISE EXCEPTION 'ECHEC - T10 tache_traitement encore présente'; END IF;
  SELECT count(*) INTO nb FROM signal WHERE id_video = v_video;
  IF nb <> 0 THEN RAISE EXCEPTION 'ECHEC - T10 signal encore présent'; END IF;
  SELECT count(*) INTO nb FROM compteur_video WHERE id_video = v_video;
  IF nb <> 0 THEN RAISE EXCEPTION 'ECHEC - T10 compteur_video encore présent'; END IF;
  SELECT count(*) INTO nb FROM vue_utilisateur WHERE id_video = v_video;
  IF nb <> 0 THEN RAISE EXCEPTION 'ECHEC - T10 vue_utilisateur encore présente'; END IF;

  RAISE NOTICE 'OK - T10 suppression utilisateur avec vidéos';
END;
$$;

-- ------------------------------------------------------------------
-- T11 : supprimer une vidéo supprime en cascade tout ce qui en dépend.
-- ------------------------------------------------------------------
DO $$
DECLARE
  v_app UUID;
  v_user UUID;
  v_video UUID;
  v_etiquette UUID;
  nb INTEGER;
BEGIN
  INSERT INTO application (nom, empreinte_cle)
  VALUES ('App T11', 'empreinte-T11') RETURNING id_application INTO v_app;
  INSERT INTO utilisateur_externe (id_application, id_externe)
  VALUES (v_app, 'u11') RETURNING id_utilisateur INTO v_user;
  INSERT INTO etiquette (id_application, nom)
  VALUES (v_app, 'E11') RETURNING id_etiquette INTO v_etiquette;
  INSERT INTO video (id_application, id_auteur, titre, cle_idempotence)
  VALUES (v_app, v_user, 'V11', 'cle-T11') RETURNING id_video INTO v_video;
  INSERT INTO fichier_video (id_video, qualite, chemin, taille_octets, format)
  VALUES (v_video, '480p', '/v11.mp4', 100, 'mp4');
  INSERT INTO tache_traitement (id_video) VALUES (v_video);
  INSERT INTO signal (id_application, id_utilisateur, id_video, type)
  VALUES (v_app, v_user, v_video, 'vue_complete');
  INSERT INTO vue_utilisateur (id_utilisateur, id_video, id_application)
  VALUES (v_user, v_video, v_app);
  INSERT INTO video_etiquette (id_video, id_etiquette, id_application)
  VALUES (v_video, v_etiquette, v_app);

  DELETE FROM video WHERE id_video = v_video;

  SELECT count(*) INTO nb FROM fichier_video WHERE id_video = v_video;
  IF nb <> 0 THEN RAISE EXCEPTION 'ECHEC - T11 fichier_video encore présent'; END IF;
  SELECT count(*) INTO nb FROM tache_traitement WHERE id_video = v_video;
  IF nb <> 0 THEN RAISE EXCEPTION 'ECHEC - T11 tache_traitement encore présente'; END IF;
  SELECT count(*) INTO nb FROM signal WHERE id_video = v_video;
  IF nb <> 0 THEN RAISE EXCEPTION 'ECHEC - T11 signal encore présent'; END IF;
  SELECT count(*) INTO nb FROM vue_utilisateur WHERE id_video = v_video;
  IF nb <> 0 THEN RAISE EXCEPTION 'ECHEC - T11 vue_utilisateur encore présente'; END IF;
  SELECT count(*) INTO nb FROM video_etiquette WHERE id_video = v_video;
  IF nb <> 0 THEN RAISE EXCEPTION 'ECHEC - T11 video_etiquette encore présente'; END IF;
  SELECT count(*) INTO nb FROM compteur_video WHERE id_video = v_video;
  IF nb <> 0 THEN RAISE EXCEPTION 'ECHEC - T11 compteur_video encore présent'; END IF;

  RAISE NOTICE 'OK - T11 suppression en cascade d''une vidéo';
END;
$$;

-- ------------------------------------------------------------------
-- T12 : supprimer_utilisateur sur un identifiant inexistant ne fait rien.
-- ------------------------------------------------------------------
DO $$
DECLARE
  v_ok BOOLEAN := FALSE;
  nb_avant INTEGER;
  nb_apres INTEGER;
BEGIN
  SELECT count(*) INTO nb_avant FROM utilisateur_externe;

  BEGIN
    PERFORM supprimer_utilisateur(
      (SELECT id_application FROM application WHERE nom = 'App A'),
      'id-inexistant'
    );
    v_ok := TRUE;
  EXCEPTION WHEN OTHERS THEN
    v_ok := FALSE;
  END;

  SELECT count(*) INTO nb_apres FROM utilisateur_externe;

  IF NOT v_ok OR nb_avant <> nb_apres THEN
    RAISE EXCEPTION 'ECHEC - T12 suppression inexistante anormale';
  END IF;

  RAISE NOTICE 'OK - T12 suppression d''un identifiant inexistant sans effet';
END;
$$;

-- ------------------------------------------------------------------
-- T13 : valeurs par défaut des trois tables principales.
-- ------------------------------------------------------------------
DO $$
DECLARE
  v_app UUID;
  v_user UUID;
  v_video UUID;
  v_role TEXT;
  v_etat TEXT;
  actif BOOLEAN;
  moderation BOOLEAN;
  creation_auto BOOLEAN;
  delai INTEGER;
  plafond_octets BIGINT;
  plafond_videos INTEGER;
  limite_req INTEGER;
BEGIN
  INSERT INTO application (nom, empreinte_cle)
  VALUES ('App T13', 'empreinte-T13') RETURNING id_application INTO v_app;

  SELECT est_active, moderation_activee, creation_auto_utilisateur,
         delai_revue_secondes, plafond_stockage_octets,
         plafond_nb_videos, limite_requetes_par_minute
    INTO actif, moderation, creation_auto, delai, plafond_octets, plafond_videos, limite_req
    FROM application WHERE id_application = v_app;

  IF actif IS DISTINCT FROM TRUE
     OR moderation IS DISTINCT FROM FALSE
     OR creation_auto IS DISTINCT FROM TRUE
     OR delai IS NOT NULL
     OR plafond_octets IS NOT NULL
     OR plafond_videos IS NOT NULL
     OR limite_req IS NOT NULL THEN
    RAISE EXCEPTION 'ECHEC - T13 valeurs par défaut de application incorrectes';
  END IF;

  INSERT INTO utilisateur_externe (id_application, id_externe)
  VALUES (v_app, 'u13') RETURNING id_utilisateur INTO v_user;

  SELECT role::text INTO v_role FROM utilisateur_externe WHERE id_utilisateur = v_user;
  IF v_role IS DISTINCT FROM 'lecteur' THEN
    RAISE EXCEPTION 'ECHEC - T13 rôle par défaut : %', v_role;
  END IF;

  INSERT INTO video (id_application, id_auteur, titre, cle_idempotence)
  VALUES (v_app, v_user, 'V13', 'cle-T13') RETURNING id_video INTO v_video;

  SELECT etat::text INTO v_etat FROM video WHERE id_video = v_video;
  IF v_etat IS DISTINCT FROM 'attente_envoi' THEN
    RAISE EXCEPTION 'ECHEC - T13 état par défaut : %', v_etat;
  END IF;

  RAISE NOTICE 'OK - T13 valeurs par défaut';
END;
$$;

-- Annulation de toutes les données de test.
ROLLBACK;
