package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"unicode/utf8"
)

// Config contient les paramètres de l'application lus depuis l'environnement.
type Config struct {
	// HTTPPort est le port d'écoute du serveur HTTP.
	HTTPPort string
	// DatabaseURL est l'adresse de connexion à la base de données.
	DatabaseURL string
	// InvitationCode est le code d'invitation de l'entreprise.
	InvitationCode string
	// AdminCode est le code réservé aux administrateurs.
	AdminCode string
	// DefaultStorageQuotaBytes est le plafond de stockage par défaut, en octets.
	// VALEURS PROVISOIRES, à confirmer par la direction de Keyavi.
	DefaultStorageQuotaBytes int64
	// DefaultMaxVideos est le nombre maximal de vidéos par défaut.
	// VALEURS PROVISOIRES, à confirmer par la direction de Keyavi.
	DefaultMaxVideos int
	// DefaultRequestsPerMinute est la limite de requêtes par minute par défaut.
	// VALEURS PROVISOIRES, à confirmer par la direction de Keyavi.
	DefaultRequestsPerMinute int
}

const (
	defautStockageOctets = int64(10737418240)
	defautMaxVideos      = 1000
	defautRequetesMinute = 600
)

// parseEntierPositif lit un entier strictement positif ; une variable vide donne la valeur par défaut.
func parseEntierPositif(nom string, defaut int64) (int64, error) {
	brut := os.Getenv(nom)
	if brut == "" {
		return defaut, nil
	}

	valeur, err := strconv.ParseInt(brut, 10, 64)
	if err != nil || valeur <= 0 {
		return 0, fmt.Errorf("%s doit être un entier strictement positif", nom)
	}
	return valeur, nil
}

// Load lit les variables d'environnement, valide les codes et les quotas et renvoie la configuration.
func Load() (Config, error) {
	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "8080"
	}

	invitation := os.Getenv("INVITATION_CODE")
	admin := os.Getenv("ADMIN_CODE")

	// Les codes sont optionnels, mais s'ils sont définis ils doivent être
	// d'au moins 16 caractères, et l'un ne doit jamais être égal à l'autre.
	if utf8.RuneCountInString(invitation) != 0 && utf8.RuneCountInString(invitation) < 16 {
		return Config{}, errors.New("INVITATION_CODE doit contenir au moins 16 caractères")
	}
	if utf8.RuneCountInString(admin) != 0 && utf8.RuneCountInString(admin) < 16 {
		return Config{}, errors.New("ADMIN_CODE doit contenir au moins 16 caractères")
	}
	if invitation != "" && admin != "" && invitation == admin {
		return Config{}, errors.New("INVITATION_CODE et ADMIN_CODE doivent être différents")
	}

	stockage, err := parseEntierPositif("DEFAULT_QUOTA_STORAGE_BYTES", defautStockageOctets)
	if err != nil {
		return Config{}, err
	}
	videos, err := parseEntierPositif("DEFAULT_QUOTA_MAX_VIDEOS", defautMaxVideos)
	if err != nil {
		return Config{}, err
	}
	requetes, err := parseEntierPositif("DEFAULT_RATE_LIMIT_PER_MINUTE", defautRequetesMinute)
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTPPort:                 port,
		DatabaseURL:              os.Getenv("DATABASE_URL"),
		InvitationCode:           invitation,
		AdminCode:                admin,
		DefaultStorageQuotaBytes: stockage,
		DefaultMaxVideos:         int(videos),
		DefaultRequestsPerMinute: int(requetes),
	}, nil
}
