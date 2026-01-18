package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aatuh/gitvault/internal/domain"
	"github.com/aatuh/gitvault/internal/services"
	"github.com/aatuh/gitvault/internal/ui"
)

type App struct {
	Out io.Writer
	Err io.Writer

	InitService   services.InitService
	DoctorService services.DoctorService
	SecretService services.SecretService
	KeysService   services.KeysService
	Listing       services.ListingService
	Sync          services.SyncService
	Store         services.VaultStore
}

func (a App) Run(ctx context.Context, args []string) int {
	global := flag.NewFlagSet("gitvault", flag.ContinueOnError)
	global.SetOutput(io.Discard)
	vaultPath := global.String("vault", "", "Vault root path")
	jsonOut := global.Bool("json", false, "Output JSON")
	help := global.Bool("help", false, "Show help")
	if err := global.Parse(args); err != nil {
		o := ui.Output{JSON: *jsonOut, Out: a.Out, Err: a.Err}
		o.Error(err)
		return 2
	}
	remaining := global.Args()
	if *help || len(remaining) == 0 {
		printUsage(a.Out)
		return 0
	}

	o := ui.Output{JSON: *jsonOut, Out: a.Out, Err: a.Err}
	cmd := remaining[0]
	switch cmd {
	case "init":
		return a.runInit(ctx, o, remaining[1:])
	case "doctor":
		root, err := a.resolveRoot(*vaultPath)
		if err != nil {
			o.Error(err)
			printVaultNotFoundHint(err, a.Err)
			return 1
		}
		return a.runDoctor(ctx, o, root, remaining[1:])
	case "secret":
		if len(remaining) == 1 || isHelpRequest(remaining[1:]) {
			return a.runSecret(ctx, o, "", remaining[1:])
		}
		root, err := a.resolveRoot(*vaultPath)
		if err != nil {
			o.Error(err)
			printVaultNotFoundHint(err, a.Err)
			return 1
		}
		return a.runSecret(ctx, o, root, remaining[1:])
	case "project":
		if isHelpRequest(remaining[1:]) {
			return a.runProject(ctx, o, "", remaining[1:])
		}
		root, err := a.resolveRoot(*vaultPath)
		if err != nil {
			o.Error(err)
			printVaultNotFoundHint(err, a.Err)
			return 1
		}
		return a.runProject(ctx, o, root, remaining[1:])
	case "env":
		if isHelpRequest(remaining[1:]) {
			return a.runEnv(ctx, o, "", remaining[1:])
		}
		root, err := a.resolveRoot(*vaultPath)
		if err != nil {
			o.Error(err)
			printVaultNotFoundHint(err, a.Err)
			return 1
		}
		return a.runEnv(ctx, o, root, remaining[1:])
	case "keys":
		if len(remaining) == 1 || isHelpRequest(remaining[1:]) {
			return a.runKeys(ctx, o, "", remaining[1:])
		}
		root, err := a.resolveRoot(*vaultPath)
		if err != nil {
			o.Error(err)
			printVaultNotFoundHint(err, a.Err)
			return 1
		}
		return a.runKeys(ctx, o, root, remaining[1:])
	case "sync":
		if len(remaining) == 1 || isHelpRequest(remaining[1:]) {
			return a.runSync(ctx, o, "", remaining[1:])
		}
		root, err := a.resolveRoot(*vaultPath)
		if err != nil {
			o.Error(err)
			printVaultNotFoundHint(err, a.Err)
			return 1
		}
		return a.runSync(ctx, o, root, remaining[1:])
	case "help":
		printUsage(a.Out)
		return 0
	default:
		o.Error(fmt.Errorf("unknown command: %s", cmd))
		printUsage(a.Err)
		return 2
	}
}

func (a App) resolveRoot(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return filepath.Abs(override)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return services.FindVaultRoot(cwd, a.Store.FS)
}

func formatWarnings(warnings []string) string {
	if len(warnings) == 0 {
		return ""
	}
	return "warnings:\n- " + strings.Join(warnings, "\n- ")
}

func parseStrategy(value string) (services.MergeStrategy, error) {
	switch value {
	case "", string(services.MergePreferVault):
		return services.MergePreferVault, nil
	case string(services.MergePreferFile):
		return services.MergePreferFile, nil
	case string(services.MergeInteractive):
		return services.MergeInteractive, nil
	default:
		return "", errors.New("invalid merge strategy")
	}
}

func validateProjectEnv(project, env string) error {
	if err := domain.ValidateIdentifier(project, "project"); err != nil {
		return err
	}
	if err := domain.ValidateIdentifier(env, "env"); err != nil {
		return err
	}
	return nil
}
