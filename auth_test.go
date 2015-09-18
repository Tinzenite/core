package core

import "testing"

func Test_StaticRandom(t *testing.T) {
	auth := Authentication{}
	err := auth.createCrypto("testtest")
	if err != nil {
		t.Error("Expected no error:", err)
	}
}
