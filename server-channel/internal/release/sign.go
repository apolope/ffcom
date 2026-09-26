package release

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrBadSignature indica assinatura que não confere com a chave pública.
var ErrBadSignature = errors.New("release: assinatura do índice inválida")

// Sign devolve a assinatura ed25519 destacada, em base64 padrão (com
// padding), sobre os bytes exatos de index. Esse é o conteúdo de
// index.json.sig.
func Sign(priv ed25519.PrivateKey, index []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, index))
}

// Verify confere sigB64 (conteúdo de index.json.sig, espaços e quebra de
// linha nas pontas são ignorados) contra os bytes exatos de index. Qualquer
// byte a mais ou a menos em index invalida a assinatura: quem baixa o índice
// deve verificar o que baixou antes de qualquer normalização.
func Verify(pub ed25519.PublicKey, index []byte, sigB64 string) error {
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("release: chave pública com %d bytes, esperado %d", len(pub), ed25519.PublicKeySize)
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigB64))
	if err != nil {
		return fmt.Errorf("%w: base64: %v", ErrBadSignature, err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("%w: %d bytes, esperado %d", ErrBadSignature, len(sig), ed25519.SignatureSize)
	}
	if !ed25519.Verify(pub, index, sig) {
		return ErrBadSignature
	}
	return nil
}

// Formato de arquivo de chave: uma linha com o base64 padrão dos bytes da
// chave, seguida de "\n". A pública tem 32 bytes; a privada tem 64 bytes
// (formato ed25519.PrivateKey do Go, semente + pública). ParsePrivateKey
// também aceita só a semente de 32 bytes.

// EncodeKey devolve o conteúdo de arquivo de uma chave (base64 + "\n").
func EncodeKey(key []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(key) + "\n")
}

// ParsePublicKey decodifica o conteúdo de um arquivo de chave pública (por
// exemplo o release.pub embutido via go:embed no lançador).
func ParsePublicKey(data []byte) (ed25519.PublicKey, error) {
	raw, err := decodeKey(data)
	if err != nil {
		return nil, fmt.Errorf("release: chave pública: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("release: chave pública com %d bytes, esperado %d", len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

// ParsePrivateKey decodifica o conteúdo de um arquivo de chave privada ou o
// valor do secret CHANNEL_RELEASE_SIGNING_KEY.
func ParsePrivateKey(data []byte) (ed25519.PrivateKey, error) {
	raw, err := decodeKey(data)
	if err != nil {
		return nil, fmt.Errorf("release: chave privada: %w", err)
	}
	switch len(raw) {
	case ed25519.PrivateKeySize:
		priv := ed25519.PrivateKey(raw)
		// Confere que a metade pública bate com a semente, para não assinar
		// com uma chave corrompida que ninguém consegue verificar.
		derived := ed25519.NewKeyFromSeed(priv.Seed())
		if !bytes.Equal(derived, priv) {
			return nil, errors.New("release: chave privada inconsistente (pública não bate com a semente)")
		}
		return priv, nil
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	default:
		return nil, fmt.Errorf("release: chave privada com %d bytes, esperado %d ou %d", len(raw), ed25519.PrivateKeySize, ed25519.SeedSize)
	}
}

func decodeKey(data []byte) ([]byte, error) {
	s := strings.TrimSpace(string(data))
	if s == "" {
		return nil, errors.New("vazia")
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("base64: %w", err)
	}
	return raw, nil
}

// ReadPublicKey lê um arquivo de chave pública.
func ReadPublicKey(path string) (ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParsePublicKey(data)
}

// ReadPrivateKey lê um arquivo de chave privada.
func ReadPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParsePrivateKey(data)
}

// WritePublicKey grava a chave pública (permissão 0644).
func WritePublicKey(path string, pub ed25519.PublicKey) error {
	return os.WriteFile(path, EncodeKey(pub), 0o644)
}

// WritePrivateKey grava a chave privada com permissão 0600 e falha se o
// arquivo já existir, para não sobrescrever uma chave em uso por engano.
// No Windows a permissão é aproximada (só o bit de somente leitura existe).
func WritePrivateKey(path string, priv ed25519.PrivateKey) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(EncodeKey(priv)); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
