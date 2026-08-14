package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	aesKeySize   = 32
	gcmNonceSize = 12
	keyIDLen     = 8
)

var (
	ErrPlaintextEmpty = errors.New("plaintext is empty")
	ErrCiphertext     = errors.New("invalid ciphertext")
	ErrUnknownKeyID   = errors.New("unknown key id")
)

// Keyring holds the current AES-256 key and optional previous keys for rotation.
type Keyring struct {
	currentID string
	keys      map[string][]byte
}

func NewKeyring(masterKey string, previous ...string) (*Keyring, error) {
	raw, err := deriveKey(masterKey)
	if err != nil {
		return nil, err
	}
	kr := &Keyring{
		currentID: keyID(raw),
		keys:      map[string][]byte{keyID(raw): raw},
	}
	for _, prev := range previous {
		prev = strings.TrimSpace(prev)
		if prev == "" {
			continue
		}
		p, err := deriveKey(prev)
		if err != nil {
			return nil, fmt.Errorf("previous master key: %w", err)
		}
		kr.keys[keyID(p)] = p
	}
	return kr, nil
}

func (k *Keyring) CurrentID() string {
	if k == nil {
		return ""
	}
	return k.currentID
}

func (k *Keyring) Encrypt(plaintext []byte) (ciphertext []byte, keyID string, err error) {
	if k == nil {
		return nil, "", errors.New("keyring is nil")
	}
	if len(plaintext) == 0 {
		return nil, "", ErrPlaintextEmpty
	}
	block, err := aes.NewCipher(k.keys[k.currentID])
	if err != nil {
		return nil, "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, "", err
	}
	nonce := make([]byte, gcmNonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, "", err
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return sealed, k.currentID, nil
}

func (k *Keyring) Decrypt(ciphertext []byte, keyID string) ([]byte, error) {
	if k == nil {
		return nil, errors.New("keyring is nil")
	}
	if len(ciphertext) < gcmNonceSize+aes.BlockSize {
		return nil, ErrCiphertext
	}
	raw, ok := k.keys[keyID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownKeyID, keyID)
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce, sealed := ciphertext[:gcmNonceSize], ciphertext[gcmNonceSize:]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, ErrCiphertext
	}
	return plain, nil
}

func deriveKey(master string) ([]byte, error) {
	master = strings.TrimSpace(master)
	if len(master) < 32 {
		return nil, fmt.Errorf("master key must be at least 32 characters")
	}
	if b, err := hex.DecodeString(master); err == nil && len(b) == aesKeySize {
		return b, nil
	}
	sum := sha256.Sum256([]byte(master))
	out := make([]byte, aesKeySize)
	copy(out, sum[:])
	return out, nil
}

func keyID(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:keyIDLen])
}
