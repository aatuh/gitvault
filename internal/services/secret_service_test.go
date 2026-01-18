package services

import (
	"context"
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

func TestApplyEnvFileUpdatesAndPreservesComments(t *testing.T) {
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
	key := "API_KEY"
	value := testutil.RandomString(t, 12)
	if err := secretSvc.Set(context.Background(), root, project, env, key, value); err != nil {
		t.Fatalf("set: %v", err)
	}
	newKey := "NEW_KEY"
	newValue := testutil.RandomString(t, 10)
	if err := secretSvc.Set(context.Background(), root, project, env, newKey, newValue); err != nil {
		t.Fatalf("set: %v", err)
	}

	path := filepath.Join(t.TempDir(), ".env")
	content := strings.Join([]string{
		"# header",
		key + "=old",
		"",
		"OTHER=keep",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	report, err := secretSvc.ApplyEnvFile(context.Background(), root, project, env, path, ApplyOptions{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if report.Updated == 0 || report.Added == 0 {
		t.Fatalf("expected updates and additions")
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	output := string(updated)
	if !strings.Contains(output, "# header") {
		t.Fatalf("expected header preserved")
	}
	if !strings.Contains(output, key+"="+value) {
		t.Fatalf("expected updated key")
	}
	if !strings.Contains(output, newKey+"="+newValue) {
		t.Fatalf("expected added key")
	}
	if !strings.Contains(output, "OTHER=keep") {
		t.Fatalf("expected existing key preserved")
	}
}

func TestApplyEnvFileOnlyExisting(t *testing.T) {
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
	key := "API_KEY"
	value := testutil.RandomString(t, 12)
	if err := secretSvc.Set(context.Background(), root, project, env, key, value); err != nil {
		t.Fatalf("set: %v", err)
	}
	newKey := "NEW_KEY"
	newValue := testutil.RandomString(t, 10)
	if err := secretSvc.Set(context.Background(), root, project, env, newKey, newValue); err != nil {
		t.Fatalf("set: %v", err)
	}

	path := filepath.Join(t.TempDir(), ".env")
	content := key + "=old\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	report, err := secretSvc.ApplyEnvFile(context.Background(), root, project, env, path, ApplyOptions{OnlyExisting: true})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if report.Added != 0 {
		t.Fatalf("expected no additions")
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	output := string(updated)
	if !strings.Contains(output, key+"="+value) {
		t.Fatalf("expected updated key")
	}
	if strings.Contains(output, newKey+"="+newValue) {
		t.Fatalf("expected new key not added")
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
