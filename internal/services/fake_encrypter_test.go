package services

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
)

type fakeEncrypter struct{}

func (fakeEncrypter) EncryptDotenv(_ context.Context, plaintext []byte, recipients []string) ([]byte, error) {
	if len(recipients) == 0 {
		return nil, errors.New("no recipients")
	}
	encoded := base64.StdEncoding.EncodeToString(plaintext)
	return []byte("ENC:" + encoded), nil
}

func (fakeEncrypter) DecryptDotenv(_ context.Context, ciphertext []byte) ([]byte, error) {
	text := string(ciphertext)
	if !strings.HasPrefix(text, "ENC:") {
		return nil, errors.New("invalid ciphertext")
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(text, "ENC:"))
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

func (fakeEncrypter) EncryptBinary(ctx context.Context, plaintext []byte, recipients []string) ([]byte, error) {
	return fakeEncrypter{}.EncryptDotenv(ctx, plaintext, recipients)
}

func (fakeEncrypter) DecryptBinary(ctx context.Context, ciphertext []byte) ([]byte, error) {
	return fakeEncrypter{}.DecryptDotenv(ctx, ciphertext)
}

func (fakeEncrypter) Version(_ context.Context) (string, error) {
	return "fake", nil
}
