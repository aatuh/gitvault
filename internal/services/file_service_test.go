package services

import (
	"context"
	"crypto/rand"
	"os"
	"testing"

	"github.com/aatuh/gitvault/internal/infra/clock"
	"github.com/aatuh/gitvault/internal/infra/fs"
	"github.com/aatuh/gitvault/internal/testutil"
)

func TestFilePutGetAndList(t *testing.T) {
	root := t.TempDir()
	filesystem := fs.OSFileSystem{}
	store := VaultStore{FS: filesystem}
	initSvc := InitService{Store: store, Clock: clock.SystemClock{}}
	if err := initSvc.Init(context.Background(), InitOptions{Root: root, Name: "test", Recipients: []string{"test"}, InitGit: false}); err != nil {
		t.Fatalf("init: %v", err)
	}

	fileSvc := FileService{Store: store, Encrypter: fakeEncrypter{}, Clock: clock.SystemClock{}}
	project := randomIdentifier(t)
	env := randomIdentifier(t)
	name := "photo-" + testutil.RandomString(t, 4) + ".jpg"

	data := make([]byte, 256)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand read: %v", err)
	}

	meta, err := fileSvc.Put(context.Background(), root, project, env, name, data)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if meta.Size != int64(len(data)) {
		t.Fatalf("expected size %d, got %d", len(data), meta.Size)
	}
	if meta.SHA256 == "" {
		t.Fatalf("expected sha256 set")
	}
	if meta.MIME == "" {
		t.Fatalf("expected mime set")
	}

	roundTrip, _, err := fileSvc.Get(context.Background(), root, project, env, name)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(roundTrip) != len(data) {
		t.Fatalf("expected %d bytes, got %d", len(data), len(roundTrip))
	}
	if string(roundTrip) != string(data) {
		t.Fatalf("expected content match")
	}

	listing := ListingService{Store: store}
	files, err := listing.ListFiles(root, project, env)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) != 1 || files[0].Name != name {
		t.Fatalf("expected file listed")
	}

	path := store.FilePath(root, project, env, name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file path present: %v", err)
	}
}
