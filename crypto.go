package core

import (
	"crypto/aes"
	"crypto/cipher"
)

type Crypto struct {
	key []byte
	gcm cipher.AEAD
}

func CreateCrypto(key []byte) (*Crypto, error) {
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(aesBlock)
	if err != nil {
		return nil, err
	}
	return &Crypto{key: key,
		gcm: gcm}, nil
}

func (c *Crypto) Encrypt(message []byte) []byte {
	/*TODO I don't yet understand all this stuff, look into message structure etc!*/
	return c.gcm.Seal(nil, []byte("noncehere!"), message, message)
}

func (c *Crypto) Decrypt(message []byte) ([]byte, error) {
	return nil, nil
}
