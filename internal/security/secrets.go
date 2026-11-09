package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

func Encrypt(key []byte, plaintext, context string) ([]byte, []byte, error) {
	if len(key) != 32 {
		return nil, nil, errors.New("encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return gcm.Seal(nil, nonce, []byte(plaintext), []byte(context)), nonce, nil
}
