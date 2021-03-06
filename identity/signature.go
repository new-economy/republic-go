package identity

import (
	"bytes"
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
)

// A Signable struct is able to be signed by a KeyPair
// Hash must return a 32-byte []byte array
type Signable interface {
	Hash() []byte
}

// The Signature type represents the signature of the hash of Signable data
type Signature = []byte

// ErrInvalidSignature indicates that a signature could not be verified against the provided Identity
var ErrInvalidSignature = fmt.Errorf("failed to verify signature")

// Sign hashes and signs the Signable data
// If the Hash() function defined does not correctly hash the struct,
// it may allow for chosen plaintext attacks on the keypair's private key
func (keyPair *KeyPair) Sign(data Signable) (Signature, error) {
	hash := data.Hash()

	return crypto.Sign(hash, keyPair.PrivateKey)
}

// RecoverSigner calculates the signing public key given signable data and its signature
func RecoverSigner(data Signable, signature Signature) (ID, error) {
	hash := data.Hash()

	// Returns 65-byte uncompress pubkey (0x04 | X | Y)
	pubkey, err := crypto.Ecrecover(hash, signature)
	if err != nil {
		return nil, err
	}

	// Convert to KeyPair before calculating ID
	id := KeyPair{
		nil, &ecdsa.PublicKey{
			Curve: secp256k1.S256(),
			X:     big.NewInt(0).SetBytes(pubkey[1:33]),
			Y:     big.NewInt(0).SetBytes(pubkey[33:65]),
		},
	}.ID()

	return id, nil
}

// VerifySignature verifies that the data's signature has been signed by the provided
// ID's private key
func VerifySignature(data Signable, signature Signature, id ID) error {
	signer, err := RecoverSigner(data, signature)
	if err != nil {
		return err
	}
	if !bytes.Equal(signer, id) {
		return ErrInvalidSignature
	}
	return nil
}
