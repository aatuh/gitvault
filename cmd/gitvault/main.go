package main

import (
	"context"
	"os"

	"github.com/aatuh/gitvault/internal/cli"
	"github.com/aatuh/sealr"
)

func main() {
	ctx := context.Background()
	system := sealr.NewDefaultSystem()

	app := cli.App{
		Out:           os.Stdout,
		Err:           os.Stderr,
		InitService:   system.InitService,
		DoctorService: system.DoctorService,
		SecretService: system.SecretService,
		FileService:   system.FileService,
		KeysService:   system.KeysService,
		Listing:       system.ListingService,
		Sync:          system.SyncService,
		Store:         system.Store,
	}

	exitCode := app.Run(ctx, os.Args[1:])
	os.Exit(exitCode)
}
