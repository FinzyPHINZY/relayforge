package security

import (
	"bytes"
	"context"
	"testing"
)

func TestKeyAndEncryption(t *testing.T) {
	raw, err := NewToken("whk_")
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := KeyPrefix(raw)
	if err != nil || len(prefix) != 16 {
		t.Fatalf("prefix=%q err=%v", prefix, err)
	}
	if !EqualHash(raw, Hash(raw)) || EqualHash(raw+"x", Hash(raw)) {
		t.Fatal("key hash verification failed")
	}
	secret, err := NewToken("whsec_")
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, nonce, err := Encrypt(bytes.Repeat([]byte{1}, 32), secret, "endpoint-id")
	if err != nil || len(ciphertext) == 0 || len(nonce) != 12 || bytes.Contains(ciphertext, []byte(secret)) {
		t.Fatal("secret encryption failed")
	}
}

func TestEndpointURLRejectsPrivateTargets(t *testing.T) {
	for _, raw := range []string{"http://8.8.8.8/x", "https://localhost/x", "https://127.0.0.1/x", "https://[::ffff:127.0.0.1]/x", "https://169.254.169.254/x", "https://192.0.2.1/x", "https://user:pass@8.8.8.8/x"} {
		if err := ValidateEndpointURL(context.Background(), raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	if err := ValidateEndpointURL(context.Background(), "https://8.8.8.8/x"); err != nil {
		t.Fatal(err)
	}
}
