-- Annulation de la migration 1 : supprime tout ce que le schéma initial a créé.
-- L'extension pgcrypto n'est volontairement pas supprimée (d'autres bases ou
-- migrations futures peuvent en dépendre).

DROP FUNCTION IF EXISTS supprimer_utilisateur(UUID, VARCHAR, BOOLEAN);
DROP TRIGGER IF EXISTS trg_creer_compteur_video ON video;
DROP FUNCTION IF EXISTS creer_compteur_video();

DROP TABLE IF EXISTS
  compteur_video,
  vue_utilisateur,
  signal,
  tache_traitement,
  video_etiquette,
  etiquette,
  fichier_video,
  video,
  utilisateur_externe,
  application;

DROP TYPE IF EXISTS qualite_video;
DROP TYPE IF EXISTS type_signal;
DROP TYPE IF EXISTS etat_tache;
DROP TYPE IF EXISTS etat_video;
DROP TYPE IF EXISTS role_utilisateur;
