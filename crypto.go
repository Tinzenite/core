package core

import (
	"crypto/aes"
	"crypto/cipher"
)

type crypto struct {
	key []byte
	gcm cipher.AEAD
}

func createCrypto(key []byte) (*crypto, error) {
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(aesBlock)
	if err != nil {
		return nil, err
	}
	return &crypto{key: key,
		gcm: gcm}, nil
}

func (c *crypto) Encrypt(message []byte) []byte {
	/*TODO I don't yet understand all this stuff, look into message structure etc!*/
	return c.gcm.Seal(nil, []byte("noncehere!"), message, message)
}

func (c *crypto) Decrypt(message []byte) ([]byte, error) {
	return nil, nil
}
