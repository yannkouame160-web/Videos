package apikey

import (
	"strings"
	"testing"
)

func TestGeneratePrefixeEtLongueur(t *testing.T) {
	cle, empreinte, err := Generate()
	if err != nil {
		t.Fatalf("Generate: erreur inattendue: %v", err)
	}
	if !strings.HasPrefix(cle, "kvk_") {
		t.Errorf("la clé doit commencer par kvk_, obtenu %q", cle)
	}
	if len(cle) != 4+64 {
		t.Errorf("longueur de la clé: attendu 68, obtenu %d", len(cle))
	}
	if len(empreinte) != 64 {
		t.Errorf("longueur de l'empreinte: attendu 64, obtenu %d", len(empreinte))
	}
}

func TestGenerateCleDifferentes(t *testing.T) {
	cle1, _, err := Generate()
	if err != nil {
		t.Fatalf("Generate: erreur inattendue: %v", err)
	}
	cle2, _, err := Generate()
	if err != nil {
		t.Fatalf("Generate: erreur inattendue: %v", err)
	}
	if cle1 == cle2 {
		t.Error("deux appels à Generate ont renvoyé la même clé")
	}
}

func TestHashDeterministe(t *testing.T) {
	empreinte1 := Hash("kvk_une-cle")
	empreinte2 := Hash("kvk_une-cle")
	if empreinte1 != empreinte2 {
		t.Error("Hash n'est pas déterministe")
	}
	if len(empreinte1) != 64 {
		t.Errorf("longueur de l'empreinte: attendu 64, obtenu %d", len(empreinte1))
	}
	if strings.ToLower(empreinte1) != empreinte1 {
		t.Error("l'empreinte doit être en hexadécimal minuscule")
	}
}

func TestGenerateEmpreinteCorrespond(t *testing.T) {
	cle, empreinte, err := Generate()
	if err != nil {
		t.Fatalf("Generate: erreur inattendue: %v", err)
	}
	if empreinte != Hash(cle) {
		t.Error("l'empreinte renvoyée par Generate ne correspond pas à Hash(cle)")
	}
}
