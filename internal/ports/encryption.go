package ports

import "context"

type Encrypter interface {
	EncryptDotenv(ctx context.Context, plaintext []byte, recipients []string) ([]byte, error)
	DecryptDotenv(ctx context.Context, ciphertext []byte) ([]byte, error)
	Version(ctx context.Context) (string, error)
}
