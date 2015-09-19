package core

import (
	"log"
	"testing"
)

func Test_Authentication(t *testing.T) {
	auth := Authentication{}
	err := auth.createCrypto("testtest")
	if err != nil {
		t.Error("Expected no error:", err)
	}
	// create new auth with Secure of old one
	twoAuth := Authentication{Secure: auth.Secure, Nonce: auth.Nonce}
	err = twoAuth.loadCrypto("testtest")
	if err != nil {
		t.Error("Expected no error:", err)
	}
	if !sameKeys(auth.public, twoAuth.public) || !sameKeys(auth.private, twoAuth.private) {
		t.Error("Expected keys to match!")
		log.Println(auth.public)
		log.Println(twoAuth.public)
		log.Println("---------------------------------------------------")
		log.Println(auth.private)
		log.Println(twoAuth.private)
	}
}

func sameKeys(a *[32]byte, b *[32]byte) bool {
	for i := 0; i < 32; i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
