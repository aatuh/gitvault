package services

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aatuh/gitvault/internal/domain"
	"github.com/aatuh/gitvault/internal/infra/clock"
	"github.com/aatuh/gitvault/internal/infra/fs"
	"github.com/aatuh/gitvault/internal/testutil"
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

func (fakeEncrypter) Version(_ context.Context) (string, error) {
	return "fake", nil
}

func TestSecretSetAndExport(t *testing.T) {
	root := t.TempDir()
	filesystem := fs.OSFileSystem{}
	store := VaultStore{FS: filesystem}
	initSvc := InitService{Store: store, Clock: clock.SystemClock{}}

	project := randomIdentifier(t)
	env := randomIdentifier(t)
	key := "API_KEY"
	value := testutil.RandomString(t, 12)

	if err := initSvc.Init(context.Background(), InitOptions{Root: root, Name: "test", Recipients: []string{"test"}, InitGit: false}); err != nil {
		t.Fatalf("init: %v", err)
	}

	secretSvc := SecretService{Store: store, Encrypter: fakeEncrypter{}, Clock: clock.SystemClock{}}
	if err := secretSvc.Set(context.Background(), root, project, env, key, value); err != nil {
		t.Fatalf("set: %v", err)
	}

	payload, err := secretSvc.ExportEnv(context.Background(), root, project, env)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	parsed, issues := domain.ParseDotenv(payload)
	for _, issue := range issues {
		if issue.Severity == domain.IssueError {
			t.Fatalf("export dotenv error: %v", issue)
		}
	}
	if parsed.Values[key] != value {
		t.Fatalf("expected %s, got %s", value, parsed.Values[key])
	}
}

func TestSecretUnsetRemovesFile(t *testing.T) {
	root := t.TempDir()
	filesystem := fs.OSFileSystem{}
	store := VaultStore{FS: filesystem}
	initSvc := InitService{Store: store, Clock: clock.SystemClock{}}
	if err := initSvc.Init(context.Background(), InitOptions{Root: root, Name: "test", Recipients: []string{"test"}, InitGit: false}); err != nil {
		t.Fatalf("init: %v", err)
	}

	secretSvc := SecretService{Store: store, Encrypter: fakeEncrypter{}, Clock: clock.SystemClock{}}
	project := randomIdentifier(t)
	env := randomIdentifier(t)
	key := "TOKEN"
	value := testutil.RandomString(t, 10)

	if err := secretSvc.Set(context.Background(), root, project, env, key, value); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := secretSvc.Unset(context.Background(), root, project, env, key); err != nil {
		t.Fatalf("unset: %v", err)
	}

	path := store.SecretFilePath(root, project, env)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected secret file removed")
	}
}

func TestImportEnvPreferFile(t *testing.T) {
	root := t.TempDir()
	filesystem := fs.OSFileSystem{}
	store := VaultStore{FS: filesystem}
	initSvc := InitService{Store: store, Clock: clock.SystemClock{}}
	if err := initSvc.Init(context.Background(), InitOptions{Root: root, Name: "test", Recipients: []string{"test"}, InitGit: false}); err != nil {
		t.Fatalf("init: %v", err)
	}
	secretSvc := SecretService{Store: store, Encrypter: fakeEncrypter{}, Clock: clock.SystemClock{}}

	project := randomIdentifier(t)
	env := randomIdentifier(t)
	key := "SERVICE_URL"
	if err := secretSvc.Set(context.Background(), root, project, env, key, "old"); err != nil {
		t.Fatalf("set: %v", err)
	}

	input := []byte(key + "=" + testutil.RandomString(t, 8) + "\n")
	report, err := secretSvc.ImportEnv(context.Background(), root, project, env, input, ImportOptions{Strategy: MergePreferFile})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if report.Updated != 1 {
		t.Fatalf("expected updated=1, got %d", report.Updated)
	}
	payload, err := secretSvc.ExportEnv(context.Background(), root, project, env)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	parsed, _ := domain.ParseDotenv(payload)
	if parsed.Values[key] == "old" {
		t.Fatalf("expected value updated")
	}
}

func randomIdentifier(t *testing.T) string {
	t.Helper()
	value := testutil.RandomString(t, 6)
	value = strings.ReplaceAll(value, "-", "")
	value = strings.ReplaceAll(value, "_", "")
	return "p" + value
}

func TestInitCreatesLayout(t *testing.T) {
	root := t.TempDir()
	filesystem := fs.OSFileSystem{}
	store := VaultStore{FS: filesystem}
	initSvc := InitService{Store: store, Clock: clock.SystemClock{}}
	if err := initSvc.Init(context.Background(), InitOptions{Root: root, Name: "test", Recipients: []string{"test"}, InitGit: false}); err != nil {
		t.Fatalf("init: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, ".gitvault")); err != nil {
		t.Fatalf("expected .gitvault: %v", err)
	}
	if _, err := os.Stat(store.SecretsDir(root)); err != nil {
		t.Fatalf("expected secrets dir: %v", err)
	}
}
