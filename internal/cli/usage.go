package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/aatuh/gitvault/internal/services"
)

func isHelpArg(arg string) bool {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "-h", "--help", "help":
		return true
	default:
		return false
	}
}

func isHelpRequest(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if isHelpArg(args[0]) {
		return true
	}
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "gitvault [--vault PATH] [--json] <command> [args]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  init           Initialize a vault repository")
	fmt.Fprintln(w, "  doctor         Verify prerequisites and key access")
	fmt.Fprintln(w, "  secret         Manage secrets (set/unset/import/export/list/find/run)")
	fmt.Fprintln(w, "  file           Store and retrieve binary files")
	fmt.Fprintln(w, "  project        List projects")
	fmt.Fprintln(w, "  env            List environments")
	fmt.Fprintln(w, "  keys           Manage recipients")
	fmt.Fprintln(w, "  sync           Git pull/push wrappers")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run `gitvault <command> --help` for details.")
}

func printSecretUsage(w io.Writer) {
	fmt.Fprintln(w, "gitvault secret <subcommand> [args]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  set         Set a key value")
	fmt.Fprintln(w, "  unset       Remove a key")
	fmt.Fprintln(w, "  import-env  Import dotenv file (alias: import)")
	fmt.Fprintln(w, "  export-env  Export dotenv file (alias: export)")
	fmt.Fprintln(w, "  apply-env   Update a dotenv file in-place (alias: apply)")
	fmt.Fprintln(w, "  list        List keys")
	fmt.Fprintln(w, "  find        Search keys")
	fmt.Fprintln(w, "  run         Run a command with env injected")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Project/env can be passed with --project/--env or as positional arguments.")
	fmt.Fprintln(w, "Flags may appear before or after positional arguments.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run `gitvault secret <subcommand> --help` for details.")
}

func printProjectUsage(w io.Writer) {
	fmt.Fprintln(w, "gitvault project list")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Projects are inferred from stored secrets.")
	fmt.Fprintln(w, "Create one by setting a secret, e.g.:")
	fmt.Fprintln(w, "  gitvault secret set <project> <env> API_KEY value")
}

func printEnvUsage(w io.Writer) {
	fmt.Fprintln(w, "gitvault env list --project <name>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Environments are inferred from stored secrets.")
	fmt.Fprintln(w, "Create one by setting a secret, e.g.:")
	fmt.Fprintln(w, "  gitvault secret set <project> <env> API_KEY value")
}

func printKeysUsage(w io.Writer) {
	fmt.Fprintln(w, "gitvault keys <list|add|remove|rotate> [args]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  gitvault keys list")
	fmt.Fprintln(w, "  gitvault keys add age1...")
	fmt.Fprintln(w, "  gitvault keys remove age1...")
	fmt.Fprintln(w, "  gitvault keys rotate")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Recipients must be age public keys (start with 'age1').")
}

func printFileUsage(w io.Writer) {
	fmt.Fprintln(w, "gitvault file <subcommand> [args]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  put    Store a binary file")
	fmt.Fprintln(w, "  get    Retrieve a binary file")
	fmt.Fprintln(w, "  list   List stored files")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Project/env can be passed with --project/--env or as positional arguments.")
	fmt.Fprintln(w, "Flags may appear before or after positional arguments.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run `gitvault file <subcommand> --help` for details.")
}

func printSyncUsage(w io.Writer) {
	fmt.Fprintln(w, "gitvault sync pull [--allow-dirty]")
	fmt.Fprintln(w, "gitvault sync push [--allow-dirty]")
}

func setInitUsage(fs *flag.FlagSet) {
	setUsage(fs,
		"gitvault init [--path <dir>] [--name <name>] [--recipient <age1...>] [--force] [--skip-git]",
		[]string{"Initializes a vault repository layout."},
		[]string{"gitvault init --path ./vault --name my-vault --recipient age1..."},
	)
}

func setDoctorUsage(fs *flag.FlagSet) {
	setUsage(fs,
		"gitvault doctor",
		[]string{"Verifies SOPS availability, key access, and decryptability."},
		nil,
	)
}

func setSecretSetUsage(fs *flag.FlagSet) {
	setUsage(fs,
		"gitvault secret set [--project <name> --env <name>] [--stdin] <project> <env> <key> <value>",
		[]string{
			"Use --stdin to read the value from standard input.",
			"Project/env can be passed with flags or positionally.",
			"Requires at least one recipient; add with `gitvault keys add age1...`.",
		},
		[]string{
			"gitvault secret set myapp dev API_KEY value",
			"gitvault secret set --project myapp --env dev API_KEY value",
		},
	)
}

func setSecretUnsetUsage(fs *flag.FlagSet) {
	setUsage(fs,
		"gitvault secret unset [--project <name> --env <name>] <project> <env> <key>",
		[]string{"Project/env can be passed with flags or positionally."},
		[]string{
			"gitvault secret unset myapp dev API_KEY",
			"gitvault secret unset --project myapp --env dev API_KEY",
		},
	)
}

func setSecretImportUsage(fs *flag.FlagSet) {
	setUsage(fs,
		"gitvault secret import-env [--project <name> --env <name>] [--file <path>] [--strategy <prefer-vault|prefer-file|interactive>] [--preserve-order|--no-preserve-order] [<project> <env>]",
		[]string{
			"Alias: gitvault secret import",
			"Project/env can be passed with flags or positionally.",
			"Preserve order keeps key order from the input file.",
		},
		[]string{
			"gitvault secret import-env --project myapp --env dev --file .env",
			"gitvault secret import-env myapp dev --file .env",
		},
	)
}

func setSecretExportUsage(fs *flag.FlagSet) {
	setUsage(fs,
		"gitvault secret export-env [--project <name> --env <name>] [--out <path|->] [--force] [--allow-git] [--preserve-order|--no-preserve-order] [<project> <env>]",
		[]string{
			"Alias: gitvault secret export",
			"Project/env can be passed with flags or positionally.",
			"Use --out - to write to stdout.",
			"Untracked files inside a git repo are allowed; tracked paths require --allow-git.",
			"Preserve order keeps key order from the vault file.",
		},
		[]string{
			"gitvault secret export-env --project myapp --env dev --out .env --force",
			"gitvault secret export-env myapp dev --out .env --force",
		},
	)
}

func setSecretListUsage(fs *flag.FlagSet) {
	setUsage(fs,
		"gitvault secret list [--project <name> --env <name>] [--show-last-changed] [<project> <env>]",
		[]string{
			"Lists keys without printing values.",
			"Project/env can be passed with flags or positionally.",
			"If no project/env is provided, lists all secret refs.",
		},
		[]string{
			"gitvault secret list --project myapp --env dev",
			"gitvault secret list myapp dev",
			"gitvault secret list",
		},
	)
}

func setSecretFindUsage(fs *flag.FlagSet) {
	setUsage(fs,
		"gitvault secret find [pattern]",
		nil,
		[]string{"gitvault secret find API"},
	)
}

func setSecretRunUsage(fs *flag.FlagSet) {
	setUsage(fs,
		"gitvault secret run [--project <name> --env <name>] [<project> <env>] -- <cmd> [args...]",
		[]string{
			"Runs a command with env injected without writing a file.",
			"Project/env can be passed with flags or positionally.",
		},
		[]string{
			"gitvault secret run --project myapp --env dev -- ./run-server",
			"gitvault secret run myapp dev -- ./run-server",
		},
	)
}

func setSecretApplyUsage(fs *flag.FlagSet) {
	setUsage(fs,
		"gitvault secret apply-env [--project <name> --env <name>] [--file <path>] [--only-existing] [--allow-git] [<project> <env>]",
		[]string{
			"Alias: gitvault secret apply",
			"Updates a dotenv file in-place using vault secrets.",
			"Project/env can be passed with flags or positionally.",
		},
		[]string{"gitvault secret apply-env --project myapp --env dev --file .env"},
	)
}

func setFilePutUsage(fs *flag.FlagSet) {
	setUsage(fs,
		"gitvault file put [--project <name> --env <name>] --path <file> [--name <name>] [<project> <env>]",
		[]string{
			"Stores the file contents encrypted in the vault.",
			"Project/env can be passed with flags or positionally.",
		},
		[]string{"gitvault file put --project myapp --env dev --path ./photo.jpg"},
	)
}

func setFileGetUsage(fs *flag.FlagSet) {
	setUsage(fs,
		"gitvault file get [--project <name> --env <name>] --name <name> [--out <path|->] [--force] [--allow-git] [<project> <env> <name>]",
		[]string{
			"Retrieves the file and writes to --out (or stdout with -).",
			"Project/env can be passed with flags or positionally.",
		},
		[]string{"gitvault file get --project myapp --env dev --name photo.jpg --out ./photo.jpg"},
	)
}

func setFileListUsage(fs *flag.FlagSet) {
	setUsage(fs,
		"gitvault file list [--project <name> --env <name>] [--show-size] [--show-last-changed] [<project> <env>]",
		[]string{
			"Lists stored file names without decrypting contents.",
			"Project/env can be passed with flags or positionally.",
		},
		[]string{
			"gitvault file list --project myapp --env dev",
			"gitvault file list",
		},
	)
}

func setSyncUsage(fs *flag.FlagSet, cmd string) {
	setUsage(fs,
		fmt.Sprintf("gitvault sync %s [--allow-dirty]", cmd),
		nil,
		nil,
	)
}

func setUsage(fs *flag.FlagSet, usageLine string, description []string, examples []string) {
	fs.Usage = func() {
		w := fs.Output()
		fmt.Fprintln(w, "Usage:")
		fmt.Fprintln(w, "  "+usageLine)
		if len(description) > 0 {
			fmt.Fprintln(w, "")
			for _, line := range description {
				fmt.Fprintln(w, line)
			}
		}
		if len(examples) > 0 {
			fmt.Fprintln(w, "")
			fmt.Fprintln(w, "Examples:")
			for _, example := range examples {
				fmt.Fprintln(w, "  "+example)
			}
		}
		if hasFlags(fs) {
			fmt.Fprintln(w, "")
			fmt.Fprintln(w, "Flags:")
			fs.PrintDefaults()
		}
	}
}

func hasFlags(fs *flag.FlagSet) bool {
	has := false
	fs.VisitAll(func(_ *flag.Flag) {
		has = true
	})
	return has
}

func printVaultNotFoundHint(err error, w io.Writer) {
	if errors.Is(err, services.ErrVaultNotFound) {
		fmt.Fprintln(w, "hint: run `gitvault init --path <vault>` or pass --vault PATH")
	}
}

func printFlagUsage(fs *flag.FlagSet, w io.Writer) {
	fs.SetOutput(w)
	fs.Usage()
}

func printSopsHint(err error, w io.Writer, json bool) {
	if json {
		return
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "sops") && !strings.Contains(msg, "age") {
		return
	}
	if strings.Contains(msg, "does not match recipients") || strings.Contains(msg, "recipient mismatch") {
		fmt.Fprintln(w, "hint: ensure your age identity matches a configured recipient")
		fmt.Fprintln(w, "hint: run `gitvault keys list` or add the correct recipient with `gitvault keys add age1...`")
		return
	}
	if strings.Contains(msg, "no identity") ||
		strings.Contains(msg, "identity") ||
		strings.Contains(msg, "failed to decrypt") ||
		strings.Contains(msg, "no key") ||
		strings.Contains(msg, "keys.txt") {
		fmt.Fprintln(w, "hint: set SOPS_AGE_KEY_FILE or run `age-keygen -o ~/.config/sops/age/keys.txt`")
		fmt.Fprintln(w, "hint: ensure the recipient is added with `gitvault keys add age1...`")
	}
}
