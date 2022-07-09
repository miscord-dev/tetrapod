package wgkey

import (
	"crypto/rand"
	"fmt"
	"io"

	"golang.org/x/crypto/nacl/box"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type DiscoPrivateKey wgtypes.Key

func New() (DiscoPrivateKey, error) {
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return DiscoPrivateKey{}, fmt.Errorf("failed to generate private key: %w", err)
	}

	return DiscoPrivateKey(key), nil
}

func (d DiscoPrivateKey) Shared(pubKey DiscoPublicKey) DiscoPrivateKey {
	var ret DiscoPrivateKey
	box.Precompute((*[32]byte)(&ret), (*[32]byte)(&d), (*[32]byte)(&pubKey))

	return ret
}

func (d DiscoPrivateKey) Public() DiscoPublicKey {
	return (DiscoPublicKey)((wgtypes.Key)(d).PublicKey())
}

type DiscoPublicKey wgtypes.Key

const (
	nonceLen = 24
)

type DiscoSharedKey wgtypes.Key

func (d DiscoSharedKey) Encrypt(cleartext []byte) (ciphertext []byte, ok bool) {
	var nonce [nonceLen]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, false
	}

	return box.SealAfterPrecomputation(nonce[:], cleartext, &nonce, (*[32]byte)(&d)), true
}

func (d DiscoSharedKey) Decrypt(ciphertext []byte) (cleartext []byte, ok bool) {
	if len(ciphertext) < nonceLen {
		return nil, false
	}

	nonce := (*[nonceLen]byte)(ciphertext)
	return box.OpenAfterPrecomputation(nil, ciphertext[nonceLen:], nonce, (*[32]byte)(&d))
}
