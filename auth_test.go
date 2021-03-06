package core

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"io/ioutil"
	"math"
	"math/big"
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
	// convert to data payload
	// NOTE: BAD DOC! binary.Size(number) does not return the correct number! Instead we use the maximum length to guarantee that the value will always fit...
	data := make([]byte, binary.MaxVarintLen64)
	_ = binary.PutVarint(data, number)
	// log.Println("GOLANG DEBUG: Size says we need", binary.Size(number), "bytes, but actually wrote", written, "!")
	// encrypt number with nonce
	encrypted, err := auth.Encrypt(data)
	if err != nil {
		t.Fatal("Expected no errors:", err)
	}
	// <-> ANSWER CHALLENGE <->
	// decrypt
	decrypted, err := auth.Decrypt(encrypted)
	if err != nil {
		t.Fatal("Expected no errors:", err)
	}
	// read number
	readNumber, err := binary.ReadVarint(bytes.NewBuffer(decrypted[:]))
	if err != nil {
		t.Fatal("Expected no errors:", err)
	}
	if readNumber != number {
		t.Error("Expected numbers to match, got", readNumber, "instead of", number)
	}
}

func Benchmark_CreateAuthentication(b *testing.B) {
	for i := 0; i < b.N; i++ {
		auth, err := createAuthentication("/path", "dirname", "username", "hunter2")
		if err != nil {
			b.Error("Error:", err)
		}
		_ = auth
	}
}

func Benchmark_LoadAuthentication(b *testing.B) {
	path, _ := ioutil.TempDir("", "auth_bench")
	auth, err := createAuthentication("/path", "dirname", "username", "hunter2")
	if err != nil {
		b.Fatal("Creation failed:", err)
	}
	err = auth.StoreTo(path)
	if err != nil {
		b.Fatal("Store failed:", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := loadAuthenticationFrom(path, "hunter2")
		if err != nil {
			b.Error("Failed to load:", err)
		}
	}
}

func Benchmark_Auth_Encrypt(b *testing.B) {
	auth, err := createAuthentication("/path", "dirname", "username", "hunter2")
	if err != nil {
		b.Fatal("Couldn't build auth:", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc, err := auth.Encrypt([]byte("Add some random test here for now."))
		if err != nil {
			b.Error("Failed to encrypt:", err)
		}
		_ = enc
	}
}

func Benchmark_Auth_Decrypt(b *testing.B) {
	auth, err := createAuthentication("/path", "dirname", "username", "hunter2")
	if err != nil {
		b.Fatal("Couldn't build auth:", err)
	}
	data := []byte("Add some random test here for now.")
	enc, err := auth.Encrypt(data)
	if err != nil {
		b.Fatal("Couldn't encrypt:", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clear, err := auth.Decrypt(enc)
		if err != nil {
			b.Error("Failed to decrypt:", err)
		}
		if 0 != bytes.Compare(clear, data) {
			b.Error("Enc and dec are different!")
		}
	}
}

func Benchmark_Auth_CreateNonce(b *testing.B) {
	auth, err := createAuthentication("/path", "dirname", "username", "hunter2")
	if err != nil {
		b.Fatal("Couldn't build auth:", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = auth.createNonce()
	}
}

func Benchmark_Auth_ConvertPassword(b *testing.B) {
	auth, err := createAuthentication("/path", "dirname", "username", "hunter2")
	if err != nil {
		b.Fatal("Couldn't build auth:", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := auth.convertPassword("hunter2")
		if err != nil {
			b.Error("Failed to build passwords:", err)
		}
	}
}

func Benchmark_Auth_CreateCrypto(b *testing.B) {
	auth, err := createAuthentication("/path", "dirname", "username", "hunter2")
	if err != nil {
		b.Fatal("Couldn't build auth:", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := auth.createCrypto("hunter2")
		if err != nil {
			b.Error("Failed to create crypto:", err)
		}
	}
}

func Benchmark_Auth_LoadCrypto(b *testing.B) {
	path, _ := ioutil.TempDir("", "auth_bench")
	auth, err := createAuthentication("/path", "dirname", "username", "hunter2")
	if err != nil {
		b.Fatal("Creation failed:", err)
	}
	err = auth.StoreTo(path)
	if err != nil {
		b.Fatal("Store failed:", err)
	}
	auth, err = loadAuthenticationFrom(path, "hunter2")
	if err != nil {
		b.Fatal("Failed to reload:", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := auth.loadCrypto("hunter2")
		if err != nil {
			b.Error("Failed to load crypto:", err)
		}
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
