package wgkey

import (
	"testing"
)

func Test_EncryptDecrypt(t *testing.T) {
	priv1, err := New()

	if err != nil {
		t.Fatal(err)
	}

	priv2, err := New()

	if err != nil {
		t.Fatal(err)
	}

	pub1 := priv1.Public()
	pub2 := priv2.Public()

	shared1 := priv1.Shared(pub2)
	shared2 := priv2.Shared(pub1)

	plaintext := []byte("Hello, world!")
	ciphertext, ok := shared1.Encrypt(plaintext)

	if !ok {
		t.Fatal("failed to encrypt")
	}

	cleartext, ok := shared2.Decrypt(ciphertext)

	if !ok {
		t.Fatal("failed to decrypt")
	}

	if string(plaintext) != string(cleartext) {
		t.Fatal("plaintext and cleartext don't match")
	}
}
