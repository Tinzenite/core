package core

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"log"
	"math"
	"math/big"
	"testing"
)

// TODO: implement benchmarking functions because encryption is slow...

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

/*
Not really a test, more an example implementation of how challenge and response
should work.
*/
func Test_Challenge(t *testing.T) {
	auth, err := createAuthentication("/path", "dirname", "username", "hunter2")
	if err != nil {
		t.Fatal("Expected no errors:", err)
	}
	// build a challenge
	bigNumber, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64-1))
	if err != nil {
		t.Fatal("Expected no errors:", err)
	}
	// convert back to int64
	number := bigNumber.Int64()
	log.Println("DEBUG: challenge:", number)
	// convert to data payload
	// NOTE: BAD DOC! binary.Size(number) does not return the correct number! Instead we use the maximum length to guarantee that the value will always fit...
	data := make([]byte, binary.MaxVarintLen64)
	written := binary.PutVarint(data, number)
	log.Println("GOLANG DEBUG: Size says we need", binary.Size(number), "bytes, but actually wrote", written, "!")
	// get a nonce
	nonce := auth.createNonce()
	// encrypt number with nonce
	encrypted, err := auth.Encrypt(data, nonce)
	if err != nil {
		t.Fatal("Expected no errors:", err)
	}
	// <-> ANSWER CHALLENGE <->
	log.Println("DEBUG: ENCRYPTED:", encrypted)
	// decrypt
	decrypted, err := auth.Decrypt(encrypted, nonce)
	if err != nil {
		t.Fatal("Expected no errors:", err)
	}
	// read number
	readNumber, err := binary.ReadVarint(bytes.NewBuffer(decrypted[:]))
	if err != nil {
		t.Fatal("Expected no errors:", err)
	}
	log.Println("DEBUG: read:", readNumber)
}

func sameKeys(a *[32]byte, b *[32]byte) bool {
	for i := 0; i < 32; i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
