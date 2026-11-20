# Note du livrable 1 : base de données et API des applications

Date : 20 novembre 2026.

## Ce qui est fait

- Base PostgreSQL 15 ou plus, créée à partir du script fourni (migration `migrations/000001_schema_initial`). Les clés étrangères composées, qui incluent l'identifiant de l'application, garantissent l'isolation entre applications au niveau de la base.
- Création d'une application avec un code d'invitation interne. La clé de l'application est générée aléatoirement et remise une seule fois ; seule son empreinte (SHA-256) est stockée.
- Vérification de la clé à chaque appel (le « portier »). Une application désactivée est refusée.
- Activation et désactivation d'une application.
- Utilisateurs externes : création automatique au premier appel (rôle lecteur), changement de rôle, suppression avec le choix de garder ou d'effacer les vidéos de l'utilisateur.

## Routes disponibles

| Route | Protection | Rôle |
|---|---|---|
| `GET /sante` | aucune | Répond « ok ». |
| `POST /v1/applications` | en-tête `X-Invitation-Code` | Crée une application et renvoie sa clé (une seule fois). |
| `GET /v1/application` | clé de l'application (`Authorization: Bearer ...`) | Renvoie l'identifiant, le nom et l'état de l'application. |
| `POST /v1/admin/applications/{id}/desactiver` | en-tête `X-Admin-Code` | Désactive une application. |
| `POST /v1/admin/applications/{id}/activer` | en-tête `X-Admin-Code` | Réactive une application. |
| `GET /v1/utilisateurs/{id_externe}` | clé de l'application | Renvoie l'utilisateur ; le crée s'il est inconnu et que la création automatique est active (201), sinon 200 ou 404. |
| `PUT /v1/utilisateurs/{id_externe}/role` | clé de l'application | Définit le rôle (lecteur, editeur, administrateur). |
| `DELETE /v1/utilisateurs/{id_externe}` | clé de l'application | Supprime l'utilisateur (`?supprimer_videos=true` pour effacer aussi ses vidéos ; par défaut elles sont gardées). |

## Comment lancer les tests

- Prérequis : PostgreSQL 15 ou plus et une base VIDE dont le nom se termine par `_test`. L'adresse de cette base est dans la variable `DATABASE_URL` du fichier `.env` (jamais dans Git).
- Tests Go : `set -a; . ./.env; set +a; go test -count=1 ./...`
- Tests SQL du schéma : `bash scripts/test_schema.sh`
- Les tests refusent de travailler sur une base dont le nom ne se termine pas par `_test` ou qui contient déjà des tables.

## Critères de réception

- Une application ne peut ni lire ni modifier les données d'une autre. Au niveau de la base : tests T1 à T4 du script SQL. Au niveau de l'API : la clé détermine l'application, jamais la requête ; un même identifiant externe dans deux applications donne deux utilisateurs distincts ; supprimer ou modifier un utilisateur dans une application ne touche pas l'autre.
- Une application désactivée est refusée (403), et redevient utilisable une fois réactivée.

## Décisions prises, à valider par la direction

1. Ajout d'un index unique sur `application.empreinte_cle` (absent du script d'origine), pour retrouver rapidement l'application à partir de sa clé.
2. Le cahier des charges ne dit pas qui peut activer ou désactiver une application. Hypothèse retenue : une route d'administration protégée par un second secret, `ADMIN_CODE`, distinct du code d'invitation.
3. Les valeurs par défaut des plafonds (stockage : 10 Go, nombre de vidéos : 1 000, requêtes par minute : 600) sont PROVISOIRES. Elles sont réglables par variables d'environnement.
4. Ajout de la route `GET /v1/application` (l'application demande ses propres informations), utile pour vérifier une clé.
5. Un `PUT` sur le rôle crée l'utilisateur s'il n'existe pas, même si la création automatique est coupée pour l'application (attribuer un rôle est une action explicite).
6. Les deux codes secrets doivent faire au moins 16 caractères et être différents ; sinon le service refuse de démarrer.
7. Les migrations s'appliquent avec `go run ./cmd/migrate up`. Il n'existe volontairement pas de commande pour revenir en arrière.
8. Les erreurs techniques sont écrites dans le journal du serveur (jamais de clé, de code ni d'adresse de base) et le client reçoit seulement « erreur interne ».

## Questions pour la direction

- Quelles sont les valeurs par défaut définitives des plafonds de stockage, de nombre de vidéos et de requêtes par minute ?
- Confirmez-vous l'hypothèse du second secret `ADMIN_CODE` pour activer et désactiver une application ?
- L'application du réseau social de l'église n'a aucun plafond : il n'existe pas de route pour cela, il suffit de mettre ces valeurs à vide dans la base. Cela convient-il ?

## Limites connues, prévues dans les livrables suivants

- La suppression d'un utilisateur avec ses vidéos efface les lignes de la base, mais pas encore les fichiers du stockage S3 : à traiter au livrable 2, AVANT l'effacement en base.
- Pas encore de limite du nombre de requêtes (le code d'invitation pourrait être deviné par essais répétés), ni de délais d'attente sur le serveur, ni de HTTPS : livrable 5.
- Les routes inconnues répondent en texte brut et non en JSON : livrable 5.
