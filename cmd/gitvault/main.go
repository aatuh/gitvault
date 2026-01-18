package main

import (
	"context"
	"os"

	"github.com/aatuh/gitvault/internal/cli"
	"github.com/aatuh/gitvault/internal/infra/clock"
	"github.com/aatuh/gitvault/internal/infra/encryption"
	executil "github.com/aatuh/gitvault/internal/infra/exec"
	"github.com/aatuh/gitvault/internal/infra/fs"
	"github.com/aatuh/gitvault/internal/infra/git"
	"github.com/aatuh/gitvault/internal/services"
)

func main() {
	ctx := context.Background()
	filesystem := fs.OSFileSystem{}
	runner := executil.ExecRunner{}
	sops := encryption.NewSops(runner)
	gitClient := git.Client{Runner: runner}
	clock := clock.SystemClock{}
	store := services.VaultStore{FS: filesystem}

	app := cli.App{
		Out:           os.Stdout,
		Err:           os.Stderr,
		InitService:   services.InitService{Store: store, Git: gitClient, Clock: clock},
		DoctorService: services.DoctorService{Store: store, Encrypter: sops, FS: filesystem},
		SecretService: services.SecretService{Store: store, Encrypter: sops, Clock: clock},
		FileService:   services.FileService{Store: store, Encrypter: sops, Clock: clock},
		KeysService:   services.KeysService{Store: store, Encrypter: sops},
		Listing:       services.ListingService{Store: store},
		Sync:          services.SyncService{Git: gitClient},
		Store:         store,
	}

	exitCode := app.Run(ctx, os.Args[1:])
	os.Exit(exitCode)
}
