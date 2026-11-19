// Package apikey génère et empreinte les clés secrètes des applications.
package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

const prefixe = "kvk_"

// Generate crée une clé aléatoire préfixée et renvoie son empreinte SHA-256.
func Generate() (cle string, empreinte string, err error) {
	octets := make([]byte, 32)
	if _, err := rand.Read(octets); err != nil {
		return "", "", err
	}

	cle = prefixe + hex.EncodeToString(octets)
	return cle, Hash(cle), nil
}

// Hash renvoie l'empreinte SHA-256 d'une clé, en hexadécimal minuscule.
func Hash(cle string) string {
	sum := sha256.Sum256([]byte(cle))
	return hex.EncodeToString(sum[:])
}
