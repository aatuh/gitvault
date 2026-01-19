package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aatuh/gitvault/internal/ui"
	"github.com/aatuh/sealr/domain"
	"github.com/aatuh/sealr/services"
)

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	*s = append(*s, value)
	return nil
}

func (a App) runInit(ctx context.Context, out ui.Output, args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(out.Out)
	setInitUsage(fs)
	path := fs.String("path", ".", "Target path")
	name := fs.String("name", "", "Vault name")
	force := fs.Bool("force", false, "Overwrite existing config")
	skipGit := fs.Bool("skip-git", false, "Skip git init")
	var recipients stringSliceFlag
	fs.Var(&recipients, "recipient", "Age recipient (repeatable)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}

	root, err := filepath.Abs(*path)
	if err != nil {
		out.Error(err)
		return 1
	}
	vaultName := strings.TrimSpace(*name)
	if vaultName == "" {
		vaultName = filepath.Base(root)
	}

	if err := a.InitService.Init(ctx, services.InitOptions{
		Root:       root,
		Name:       vaultName,
		Recipients: recipients,
		Force:      *force,
		InitGit:    !*skipGit,
	}); err != nil {
		out.Error(err)
		return 1
	}

	warning := ""
	if len(recipients) == 0 {
		warning = "no recipients configured; add one with `gitvault keys add age1...` before setting secrets"
	}
	if out.JSON {
		data := map[string]string{"root": root}
		if warning != "" {
			data["warning"] = warning
		}
		out.Success("vault initialized", data)
		return 0
	}
	fmt.Fprintln(out.Out, "vault initialized")
	fmt.Fprintf(out.Out, "root: %s\n", root)
	fmt.Fprintln(out.Out, "created:")
	fmt.Fprintf(out.Out, "  %s\n", filepath.Join(root, ".gitvault"))
	fmt.Fprintf(out.Out, "  %s\n", filepath.Join(root, "secrets"))
	fmt.Fprintf(out.Out, "  %s\n", filepath.Join(root, "files"))
	fmt.Fprintln(out.Out, "next:")
	fmt.Fprintf(out.Out, "  gitvault --vault %s doctor\n", root)
	fmt.Fprintf(out.Out, "  gitvault --vault %s keys add <age1...>\n", root)
	fmt.Fprintf(out.Out, "  gitvault --vault %s secret set <project> <env> KEY value\n", root)
	if warning != "" {
		fmt.Fprintln(out.Out, "note:", warning)
	}
	return 0
}

func (a App) runDoctor(ctx context.Context, out ui.Output, root string, args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(out.Out)
	setDoctorUsage(fs)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}

	report, err := a.DoctorService.Run(ctx, root)
	if err != nil {
		out.Error(err)
		return 1
	}

	rows := make([][]string, 0, len(report.Checks))
	for _, check := range report.Checks {
		rows = append(rows, []string{check.Name, string(check.Status), check.Message})
	}
	out.Table([]string{"check", "status", "message"}, rows)
	for _, check := range report.Checks {
		if check.Name == "vault config" && check.Status == services.CheckFail {
			fmt.Fprintln(out.Err, "hint: run `gitvault init --path <vault>` or pass --vault PATH")
		}
		if check.Name == "age identity" && check.Status != services.CheckOK {
			fmt.Fprintln(out.Err, "hint: set SOPS_AGE_KEY_FILE or run `age-keygen -o ~/.config/sops/age/keys.txt`")
		}
	}
	if report.HasFailures() {
		return 1
	}
	return 0
}

func (a App) runSecret(ctx context.Context, out ui.Output, root string, args []string) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printSecretUsage(out.Out)
		return 0
	}
	switch args[0] {
	case "set":
		return a.runSecretSet(ctx, out, root, args[1:])
	case "unset":
		return a.runSecretUnset(ctx, out, root, args[1:])
	case "import-env", "import":
		return a.runSecretImport(ctx, out, root, args[1:])
	case "export-env", "export":
		return a.runSecretExport(ctx, out, root, args[1:])
	case "apply-env", "apply":
		return a.runSecretApply(ctx, out, root, args[1:])
	case "list":
		return a.runSecretList(ctx, out, root, args[1:])
	case "find":
		return a.runSecretFind(ctx, out, root, args[1:])
	case "run":
		return a.runSecretRun(ctx, out, root, args[1:])
	default:
		out.Error(fmt.Errorf("unknown secret subcommand: %s", args[0]))
		printSecretUsage(out.Err)
		return 2
	}
}

func (a App) runSecretSet(ctx context.Context, out ui.Output, root string, args []string) int {
	fs := flag.NewFlagSet("secret set", flag.ContinueOnError)
	fs.SetOutput(out.Out)
	setSecretSetUsage(fs)
	project := fs.String("project", "", "Project name")
	env := fs.String("env", "", "Environment name")
	stdin := fs.Bool("stdin", false, "Read value from stdin")
	if err := parseFlagSet(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}

	remaining, err := fillProjectEnv(project, env, fs.Args())
	if err != nil {
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	if *project == "" || *env == "" {
		out.Error(errors.New("--project and --env are required"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if len(remaining) < 1 {
		out.Error(errors.New("key is required"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if !*stdin && len(remaining) < 2 {
		out.Error(errors.New("value is required (or use --stdin)"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if *stdin && len(remaining) > 2 {
		out.Error(errors.New("unexpected extra arguments"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if !*stdin && len(remaining) > 2 {
		out.Error(errors.New("unexpected extra arguments"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	key := remaining[0]
	value := ""
	if len(remaining) > 1 {
		value = remaining[1]
	}
	if *stdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			out.Error(err)
			return 1
		}
		value = strings.TrimRight(string(data), "\n")
	}

	if err := a.SecretService.Set(ctx, root, *project, *env, key, value); err != nil {
		out.Error(err)
		printSopsHint(err, out.Err, out.JSON)
		return 1
	}
	out.Success("secret updated", map[string]string{"project": *project, "env": *env, "key": key})
	return 0
}

func (a App) runSecretUnset(ctx context.Context, out ui.Output, root string, args []string) int {
	fs := flag.NewFlagSet("secret unset", flag.ContinueOnError)
	fs.SetOutput(out.Out)
	setSecretUnsetUsage(fs)
	project := fs.String("project", "", "Project name")
	env := fs.String("env", "", "Environment name")
	if err := parseFlagSet(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	remaining, err := fillProjectEnv(project, env, fs.Args())
	if err != nil {
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	if *project == "" || *env == "" {
		out.Error(errors.New("--project and --env are required"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if len(remaining) < 1 {
		out.Error(errors.New("key is required"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if len(remaining) > 1 {
		out.Error(errors.New("unexpected extra arguments"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	key := remaining[0]
	if err := a.SecretService.Unset(ctx, root, *project, *env, key); err != nil {
		out.Error(err)
		printSopsHint(err, out.Err, out.JSON)
		return 1
	}
	out.Success("secret removed", map[string]string{"project": *project, "env": *env, "key": key})
	return 0
}

func (a App) runSecretImport(ctx context.Context, out ui.Output, root string, args []string) int {
	fs := flag.NewFlagSet("secret import-env", flag.ContinueOnError)
	fs.SetOutput(out.Out)
	setSecretImportUsage(fs)
	project := fs.String("project", "", "Project name")
	env := fs.String("env", "", "Environment name")
	file := fs.String("file", ".env", "Dotenv file path")
	strategy := fs.String("strategy", string(services.MergePreferVault), "Merge strategy")
	preserveOrder := fs.Bool("preserve-order", true, "Preserve key order from input file")
	noPreserveOrder := fs.Bool("no-preserve-order", false, "Sort keys instead of preserving order")
	if err := parseFlagSet(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	remaining, err := fillProjectEnv(project, env, fs.Args())
	if err != nil {
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	if len(remaining) > 0 {
		out.Error(errors.New("unexpected extra arguments"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if *project == "" || *env == "" {
		out.Error(errors.New("--project and --env are required"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	mergeStrategy, err := parseStrategy(*strategy)
	if err != nil {
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}

	data, err := os.ReadFile(*file)
	if err != nil {
		out.Error(err)
		return 1
	}

	var resolver services.ConflictResolver
	if mergeStrategy == services.MergeInteractive {
		resolver = func(key, vaultValue, fileValue string) (string, error) {
			prompt := fmt.Sprintf("conflict for %s (vault=%s, file=%s). choose [v]ault/[f]ile: ", key, vaultValue, fileValue)
			fmt.Fprint(out.Out, prompt)
			reader := bufio.NewReader(os.Stdin)
			answer, err := reader.ReadString('\n')
			if err != nil {
				return "", err
			}
			answer = strings.ToLower(strings.TrimSpace(answer))
			if answer == "f" {
				return fileValue, nil
			}
			return vaultValue, nil
		}
	}

	usePreserveOrder := *preserveOrder && !*noPreserveOrder
	report, err := a.SecretService.ImportEnv(ctx, root, *project, *env, data, services.ImportOptions{
		Strategy:        mergeStrategy,
		Resolver:        resolver,
		NoPreserveOrder: !usePreserveOrder,
	})
	if err != nil {
		out.Error(err)
		printSopsHint(err, out.Err, out.JSON)
		return 1
	}

	payload := map[string]interface{}{
		"added":   report.Added,
		"updated": report.Updated,
		"skipped": report.Skipped,
	}
	if len(report.Warnings) > 0 {
		payload["warnings"] = report.Warnings
	}
	out.Success("import complete", payload)
	return 0
}

func (a App) runSecretExport(ctx context.Context, out ui.Output, root string, args []string) int {
	fs := flag.NewFlagSet("secret export-env", flag.ContinueOnError)
	fs.SetOutput(out.Out)
	setSecretExportUsage(fs)
	project := fs.String("project", "", "Project name")
	env := fs.String("env", "", "Environment name")
	outPath := fs.String("out", "-", "Output path or - for stdout")
	force := fs.Bool("force", false, "Overwrite output file")
	allowGit := fs.Bool("allow-git", false, "Allow writing into git-tracked paths")
	preserveOrder := fs.Bool("preserve-order", true, "Preserve key order from vault")
	noPreserveOrder := fs.Bool("no-preserve-order", false, "Sort keys instead of preserving order")
	if err := parseFlagSet(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	remaining, err := fillProjectEnv(project, env, fs.Args())
	if err != nil {
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	if len(remaining) > 0 {
		out.Error(errors.New("unexpected extra arguments"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if *project == "" || *env == "" {
		out.Error(errors.New("--project and --env are required"))
		printFlagUsage(fs, out.Err)
		return 2
	}

	usePreserveOrder := *preserveOrder && !*noPreserveOrder
	payload, err := a.SecretService.ExportEnvWithOptions(ctx, root, *project, *env, services.ExportOptions{NoPreserveOrder: !usePreserveOrder})
	if err != nil {
		out.Error(err)
		printSopsHint(err, out.Err, out.JSON)
		return 1
	}

	if *outPath == "-" {
		_, _ = out.Out.Write(payload)
		return 0
	}

	if err := a.guardOutputPath(ctx, root, *outPath, *allowGit, *force); err != nil {
		out.Error(err)
		return 1
	}
	if err := writeEnvFile(*outPath, payload); err != nil {
		out.Error(err)
		return 1
	}
	out.Success("exported", map[string]string{"path": *outPath})
	return 0
}

func (a App) runSecretApply(ctx context.Context, out ui.Output, root string, args []string) int {
	fs := flag.NewFlagSet("secret apply-env", flag.ContinueOnError)
	fs.SetOutput(out.Out)
	setSecretApplyUsage(fs)
	project := fs.String("project", "", "Project name")
	env := fs.String("env", "", "Environment name")
	file := fs.String("file", ".env", "Dotenv file path")
	onlyExisting := fs.Bool("only-existing", false, "Only update keys already present in the file")
	allowGit := fs.Bool("allow-git", false, "Allow updating git-tracked files")
	if err := parseFlagSet(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	remaining, err := fillProjectEnv(project, env, fs.Args())
	if err != nil {
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	if len(remaining) > 0 {
		out.Error(errors.New("unexpected extra arguments"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if *project == "" || *env == "" {
		out.Error(errors.New("--project and --env are required"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if strings.TrimSpace(*file) == "" {
		out.Error(errors.New("--file is required"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if _, err := os.Stat(*file); err != nil {
		out.Error(err)
		return 1
	}
	if err := a.guardUpdatePath(ctx, root, *file, *allowGit); err != nil {
		out.Error(err)
		return 1
	}
	report, err := a.SecretService.ApplyEnvFile(ctx, root, *project, *env, *file, services.ApplyOptions{OnlyExisting: *onlyExisting})
	if err != nil {
		out.Error(err)
		printSopsHint(err, out.Err, out.JSON)
		return 1
	}
	payload := map[string]interface{}{
		"path":    *file,
		"updated": report.Updated,
		"added":   report.Added,
	}
	out.Success("apply complete", payload)
	return 0
}

func (a App) runSecretList(ctx context.Context, out ui.Output, root string, args []string) int {
	fs := flag.NewFlagSet("secret list", flag.ContinueOnError)
	fs.SetOutput(out.Out)
	setSecretListUsage(fs)
	project := fs.String("project", "", "Project name")
	env := fs.String("env", "", "Environment name")
	showChanged := fs.Bool("show-last-changed", false, "Show last updated time")
	if err := parseFlagSet(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	remaining, err := fillProjectEnv(project, env, fs.Args())
	if err != nil {
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	if len(remaining) > 0 {
		out.Error(errors.New("unexpected extra arguments"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if *project == "" && *env == "" {
		keys, err := a.Listing.ListAllKeys(root)
		if err != nil {
			out.Error(err)
			return 1
		}
		if len(keys) == 0 {
			if out.JSON {
				out.Table([]string{"ref"}, nil)
			} else {
				fmt.Fprintln(out.Out, "no secrets yet")
				fmt.Fprintln(out.Out, "hint: add one with `gitvault secret set <project> <env> KEY value`")
			}
			return 0
		}
		if out.JSON {
			rows := make([][]string, 0, len(keys))
			for _, key := range keys {
				row := []string{key.Name}
				if *showChanged {
					if key.LastUpdated.IsZero() {
						row = append(row, "")
					} else {
						row = append(row, key.LastUpdated.Format("2006-01-02T15:04:05Z"))
					}
				}
				rows = append(rows, row)
			}
			headers := []string{"ref"}
			if *showChanged {
				headers = append(headers, "last_updated")
			}
			out.Table(headers, rows)
			return 0
		}
		rows := make([][]string, 0, len(keys))
		for _, key := range keys {
			projectName, envName, keyName := splitKeyRef(key.Name)
			row := []string{projectName, envName, keyName}
			if *showChanged {
				if key.LastUpdated.IsZero() {
					row = append(row, "")
				} else {
					row = append(row, key.LastUpdated.Format("2006-01-02T15:04:05Z"))
				}
			}
			rows = append(rows, row)
		}
		headers := []string{"project", "env", "key"}
		if *showChanged {
			headers = append(headers, "last_updated")
		}
		out.Table(headers, rows)
		return 0
	}
	if *project == "" || *env == "" {
		out.Error(errors.New("--project and --env are required"))
		printFlagUsage(fs, out.Err)
		fmt.Fprintln(out.Err, "hint: use `gitvault project list` and `gitvault env list --project <name>`")
		return 2
	}
	keys, err := a.Listing.ListKeys(root, *project, *env)
	if err != nil {
		out.Error(err)
		return 1
	}
	if len(keys) == 0 {
		if out.JSON {
			out.Table([]string{"key"}, nil)
		} else {
			fmt.Fprintf(out.Out, "no secrets for %s/%s\n", *project, *env)
			fmt.Fprintln(out.Out, "hint: add one with `gitvault secret set <project> <env> KEY value`")
		}
		return 0
	}
	rows := make([][]string, 0, len(keys))
	for _, key := range keys {
		row := []string{key.Name}
		if !out.JSON {
			row = []string{*project, *env, key.Name}
		}
		if *showChanged {
			if key.LastUpdated.IsZero() {
				row = append(row, "")
			} else {
				row = append(row, key.LastUpdated.Format("2006-01-02T15:04:05Z"))
			}
		}
		rows = append(rows, row)
	}
	headers := []string{"key"}
	if !out.JSON {
		headers = []string{"project", "env", "key"}
	}
	if *showChanged {
		headers = append(headers, "last_updated")
	}
	out.Table(headers, rows)
	return 0
}

func (a App) runSecretFind(ctx context.Context, out ui.Output, root string, args []string) int {
	fs := flag.NewFlagSet("secret find", flag.ContinueOnError)
	fs.SetOutput(out.Out)
	setSecretFindUsage(fs)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	pattern := ""
	if len(fs.Args()) > 0 {
		pattern = fs.Args()[0]
	}
	matches, err := a.Listing.FindKeys(root, pattern)
	if err != nil {
		out.Error(err)
		return 1
	}
	rows := make([][]string, 0, len(matches))
	for _, ref := range matches {
		rows = append(rows, []string{ref})
	}
	out.Table([]string{"ref"}, rows)
	return 0
}

func (a App) runSecretRun(ctx context.Context, out ui.Output, root string, args []string) int {
	fs := flag.NewFlagSet("secret run", flag.ContinueOnError)
	fs.SetOutput(out.Out)
	setSecretRunUsage(fs)
	project := fs.String("project", "", "Project name")
	env := fs.String("env", "", "Environment name")
	if err := parseFlagSet(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	remaining, err := fillProjectEnv(project, env, fs.Args())
	if err != nil {
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	if *project == "" || *env == "" {
		out.Error(errors.New("--project and --env are required"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if len(remaining) > 0 && remaining[0] == "--" {
		remaining = remaining[1:]
	}
	cmdArgs := remaining
	if len(cmdArgs) == 0 {
		out.Error(errors.New("command required after flags"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	payload, err := a.SecretService.ExportEnv(ctx, root, *project, *env)
	if err != nil {
		out.Error(err)
		printSopsHint(err, out.Err, out.JSON)
		return 1
	}
	parsed, issues := domain.ParseDotenv(payload)
	for _, issue := range issues {
		if issue.Severity == domain.IssueError {
			out.Error(fmt.Errorf("dotenv parse error: %s", issue.Message))
			return 1
		}
	}

	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = append(os.Environ(), flattenEnv(parsed.Values)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = out.Out
	cmd.Stderr = out.Err
	if err := cmd.Run(); err != nil {
		out.Error(err)
		return 1
	}
	return 0
}

func (a App) runProject(ctx context.Context, out ui.Output, root string, args []string) int {
	fs := flag.NewFlagSet("project", flag.ContinueOnError)
	fs.SetOutput(out.Out)
	if len(args) > 1 && args[0] == "list" && isHelpArg(args[1]) {
		printProjectUsage(out.Out)
		return 0
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printProjectUsage(out.Out)
			return 0
		}
		out.Error(err)
		printProjectUsage(out.Err)
		return 2
	}
	if len(fs.Args()) > 0 && fs.Args()[0] != "list" {
		out.Error(errors.New("unknown project subcommand"))
		printProjectUsage(out.Err)
		return 2
	}
	if len(fs.Args()) > 1 {
		out.Error(errors.New("unexpected extra arguments"))
		printProjectUsage(out.Err)
		return 2
	}
	projects, err := a.Listing.ListProjects(root)
	if err != nil {
		out.Error(err)
		return 1
	}
	if len(projects) == 0 {
		if out.JSON {
			out.Table([]string{"project"}, nil)
		} else {
			fmt.Fprintln(out.Out, "no projects yet")
			fmt.Fprintln(out.Out, "hint: add one with `gitvault secret set <project> <env> KEY value`")
		}
		return 0
	}
	rows := make([][]string, 0, len(projects))
	for _, project := range projects {
		rows = append(rows, []string{project})
	}
	out.Table([]string{"project"}, rows)
	return 0
}

func (a App) runEnv(ctx context.Context, out ui.Output, root string, args []string) int {
	fs := flag.NewFlagSet("env", flag.ContinueOnError)
	fs.SetOutput(out.Out)
	project := fs.String("project", "", "Project name")
	if len(args) > 0 && args[0] == "list" {
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printEnvUsage(out.Out)
			return 0
		}
		out.Error(err)
		printEnvUsage(out.Err)
		return 2
	}
	if *project == "" {
		out.Error(errors.New("--project is required"))
		printEnvUsage(out.Err)
		return 2
	}
	if len(fs.Args()) > 0 {
		out.Error(errors.New("unknown env subcommand"))
		printEnvUsage(out.Err)
		return 2
	}
	envs, err := a.Listing.ListEnvs(root, *project)
	if err != nil {
		out.Error(err)
		return 1
	}
	if len(envs) == 0 {
		if out.JSON {
			out.Table([]string{"env"}, nil)
		} else {
			fmt.Fprintf(out.Out, "no environments for %s yet\n", *project)
			fmt.Fprintln(out.Out, "hint: add one with `gitvault secret set <project> <env> KEY value`")
		}
		return 0
	}
	rows := make([][]string, 0, len(envs))
	for _, env := range envs {
		rows = append(rows, []string{env})
	}
	out.Table([]string{"env"}, rows)
	return 0
}

func (a App) runKeys(ctx context.Context, out ui.Output, root string, args []string) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printKeysUsage(out.Out)
		return 0
	}
	cmd := args[0]
	switch cmd {
	case "list":
		keys, err := a.KeysService.List(root)
		if err != nil {
			out.Error(err)
			return 1
		}
		rows := make([][]string, 0, len(keys))
		for _, key := range keys {
			rows = append(rows, []string{key})
		}
		out.Table([]string{"recipient"}, rows)
		return 0
	case "add":
		if len(args) >= 2 && isHelpArg(args[1]) {
			printKeysUsage(out.Out)
			return 0
		}
		if len(args) < 2 {
			out.Error(errors.New("recipient is required"))
			printKeysUsage(out.Err)
			return 2
		}
		if err := a.KeysService.Add(root, args[1]); err != nil {
			out.Error(err)
			return 1
		}
		out.Success("recipient added", map[string]string{"recipient": args[1]})
		return 0
	case "remove":
		if len(args) >= 2 && isHelpArg(args[1]) {
			printKeysUsage(out.Out)
			return 0
		}
		if len(args) < 2 {
			out.Error(errors.New("recipient is required"))
			printKeysUsage(out.Err)
			return 2
		}
		if err := a.KeysService.Remove(root, args[1]); err != nil {
			out.Error(err)
			return 1
		}
		out.Success("recipient removed", map[string]string{"recipient": args[1]})
		return 0
	case "rotate":
		report, err := a.KeysService.Rotate(ctx, root)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				out.Success("no secrets to rotate", nil)
				return 0
			}
			out.Error(err)
			printSopsHint(err, out.Err, out.JSON)
			return 1
		}
		payload := map[string]interface{}{
			"total":   report.Total,
			"rotated": report.Rotated,
			"failed":  report.Failed,
		}
		if len(report.Errors) > 0 {
			payload["errors"] = report.Errors
		}
		out.Success("rotation complete", payload)
		if report.Failed > 0 {
			return 1
		}
		return 0
	default:
		out.Error(fmt.Errorf("unknown keys subcommand: %s", cmd))
		printKeysUsage(out.Err)
		return 2
	}
}

func (a App) runSync(ctx context.Context, out ui.Output, root string, args []string) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printSyncUsage(out.Out)
		return 0
	}
	cmd := args[0]
	fs := flag.NewFlagSet("sync "+cmd, flag.ContinueOnError)
	fs.SetOutput(out.Out)
	setSyncUsage(fs, cmd)
	allowDirty := fs.Bool("allow-dirty", false, "Allow dirty working tree")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	switch cmd {
	case "pull":
		if err := a.Sync.Pull(ctx, root, *allowDirty); err != nil {
			out.Error(err)
			return 1
		}
		out.Success("pulled", nil)
		return 0
	case "push":
		if err := a.Sync.Push(ctx, root, *allowDirty); err != nil {
			out.Error(err)
			return 1
		}
		out.Success("pushed", nil)
		return 0
	default:
		out.Error(fmt.Errorf("unknown sync subcommand: %s", cmd))
		printSyncUsage(out.Err)
		return 2
	}
}

func (a App) runFile(ctx context.Context, out ui.Output, root string, args []string) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printFileUsage(out.Out)
		return 0
	}
	switch args[0] {
	case "put":
		return a.runFilePut(ctx, out, root, args[1:])
	case "get":
		return a.runFileGet(ctx, out, root, args[1:])
	case "list":
		return a.runFileList(ctx, out, root, args[1:])
	default:
		out.Error(fmt.Errorf("unknown file subcommand: %s", args[0]))
		printFileUsage(out.Err)
		return 2
	}
}

func (a App) runFilePut(ctx context.Context, out ui.Output, root string, args []string) int {
	fs := flag.NewFlagSet("file put", flag.ContinueOnError)
	fs.SetOutput(out.Out)
	setFilePutUsage(fs)
	project := fs.String("project", "", "Project name")
	env := fs.String("env", "", "Environment name")
	path := fs.String("path", "", "Input file path")
	name := fs.String("name", "", "File name to store (defaults to base name of --path)")
	if err := parseFlagSet(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	remaining, err := fillProjectEnv(project, env, fs.Args())
	if err != nil {
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	if len(remaining) > 0 {
		out.Error(errors.New("unexpected extra arguments"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if *project == "" || *env == "" {
		out.Error(errors.New("--project and --env are required"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if strings.TrimSpace(*path) == "" {
		out.Error(errors.New("--path is required"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	info, err := os.Stat(*path)
	if err != nil {
		out.Error(err)
		return 1
	}
	if info.IsDir() {
		out.Error(errors.New("path must be a file"))
		return 1
	}
	if strings.TrimSpace(*name) == "" {
		*name = filepath.Base(*path)
	}
	data, err := os.ReadFile(*path)
	if err != nil {
		out.Error(err)
		return 1
	}
	meta, err := a.FileService.Put(ctx, root, *project, *env, *name, data)
	if err != nil {
		out.Error(err)
		printSopsHint(err, out.Err, out.JSON)
		return 1
	}
	payload := map[string]interface{}{
		"project": *project,
		"env":     *env,
		"name":    *name,
		"size":    meta.Size,
		"sha256":  meta.SHA256,
	}
	out.Success("file stored", payload)
	return 0
}

func (a App) runFileGet(ctx context.Context, out ui.Output, root string, args []string) int {
	fs := flag.NewFlagSet("file get", flag.ContinueOnError)
	fs.SetOutput(out.Out)
	setFileGetUsage(fs)
	project := fs.String("project", "", "Project name")
	env := fs.String("env", "", "Environment name")
	name := fs.String("name", "", "File name to retrieve")
	outPath := fs.String("out", "-", "Output path or - for stdout")
	force := fs.Bool("force", false, "Overwrite output file")
	allowGit := fs.Bool("allow-git", false, "Allow writing into git-tracked paths")
	if err := parseFlagSet(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	remaining, err := fillProjectEnv(project, env, fs.Args())
	if err != nil {
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	if *name == "" && len(remaining) > 0 {
		*name = remaining[0]
		remaining = remaining[1:]
	}
	if len(remaining) > 0 {
		out.Error(errors.New("unexpected extra arguments"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if *project == "" || *env == "" {
		out.Error(errors.New("--project and --env are required"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if strings.TrimSpace(*name) == "" {
		out.Error(errors.New("--name is required"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	payload, _, err := a.FileService.Get(ctx, root, *project, *env, *name)
	if err != nil {
		out.Error(err)
		printSopsHint(err, out.Err, out.JSON)
		return 1
	}
	if *outPath == "-" {
		_, _ = out.Out.Write(payload)
		return 0
	}
	if err := a.guardOutputPath(ctx, root, *outPath, *allowGit, *force); err != nil {
		out.Error(err)
		return 1
	}
	if err := writeBinaryFile(*outPath, payload); err != nil {
		out.Error(err)
		return 1
	}
	out.Success("file retrieved", map[string]string{"path": *outPath})
	return 0
}

func (a App) runFileList(ctx context.Context, out ui.Output, root string, args []string) int {
	fs := flag.NewFlagSet("file list", flag.ContinueOnError)
	fs.SetOutput(out.Out)
	setFileListUsage(fs)
	project := fs.String("project", "", "Project name")
	env := fs.String("env", "", "Environment name")
	showChanged := fs.Bool("show-last-changed", false, "Show last updated time")
	showSize := fs.Bool("show-size", false, "Show file size")
	if err := parseFlagSet(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	remaining, err := fillProjectEnv(project, env, fs.Args())
	if err != nil {
		out.Error(err)
		printFlagUsage(fs, out.Err)
		return 2
	}
	if len(remaining) > 0 {
		out.Error(errors.New("unexpected extra arguments"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	if *project == "" && *env == "" {
		files, err := a.Listing.ListAllFiles(root)
		if err != nil {
			out.Error(err)
			return 1
		}
		if len(files) == 0 {
			if out.JSON {
				out.Table([]string{"ref"}, nil)
			} else {
				fmt.Fprintln(out.Out, "no files yet")
				fmt.Fprintln(out.Out, "hint: add one with `gitvault file put <project> <env> --path <file>`")
			}
			return 0
		}
		if out.JSON {
			rows := make([][]string, 0, len(files))
			for _, file := range files {
				row := []string{file.Name}
				if *showSize {
					row = append(row, fmt.Sprintf("%d", file.Size))
				}
				if *showChanged {
					if file.LastUpdated.IsZero() {
						row = append(row, "")
					} else {
						row = append(row, file.LastUpdated.Format("2006-01-02T15:04:05Z"))
					}
				}
				rows = append(rows, row)
			}
			headers := []string{"ref"}
			if *showSize {
				headers = append(headers, "size")
			}
			if *showChanged {
				headers = append(headers, "last_updated")
			}
			out.Table(headers, rows)
			return 0
		}
		rows := make([][]string, 0, len(files))
		for _, file := range files {
			projectName, envName, fileName := splitKeyRef(file.Name)
			row := []string{projectName, envName, fileName}
			if *showSize {
				row = append(row, fmt.Sprintf("%d", file.Size))
			}
			if *showChanged {
				if file.LastUpdated.IsZero() {
					row = append(row, "")
				} else {
					row = append(row, file.LastUpdated.Format("2006-01-02T15:04:05Z"))
				}
			}
			rows = append(rows, row)
		}
		headers := []string{"project", "env", "file"}
		if *showSize {
			headers = append(headers, "size")
		}
		if *showChanged {
			headers = append(headers, "last_updated")
		}
		out.Table(headers, rows)
		return 0
	}
	if *project == "" || *env == "" {
		out.Error(errors.New("--project and --env are required"))
		printFlagUsage(fs, out.Err)
		return 2
	}
	files, err := a.Listing.ListFiles(root, *project, *env)
	if err != nil {
		out.Error(err)
		return 1
	}
	if len(files) == 0 {
		if out.JSON {
			out.Table([]string{"file"}, nil)
		} else {
			fmt.Fprintf(out.Out, "no files for %s/%s\n", *project, *env)
			fmt.Fprintln(out.Out, "hint: add one with `gitvault file put <project> <env> --path <file>`")
		}
		return 0
	}
	rows := make([][]string, 0, len(files))
	for _, file := range files {
		row := []string{file.Name}
		if !out.JSON {
			row = []string{*project, *env, file.Name}
		}
		if *showSize {
			row = append(row, fmt.Sprintf("%d", file.Size))
		}
		if *showChanged {
			if file.LastUpdated.IsZero() {
				row = append(row, "")
			} else {
				row = append(row, file.LastUpdated.Format("2006-01-02T15:04:05Z"))
			}
		}
		rows = append(rows, row)
	}
	headers := []string{"file"}
	if !out.JSON {
		headers = []string{"project", "env", "file"}
	}
	if *showSize {
		headers = append(headers, "size")
	}
	if *showChanged {
		headers = append(headers, "last_updated")
	}
	out.Table(headers, rows)
	return 0
}

func (a App) guardOutputPath(ctx context.Context, root, outPath string, allowGit bool, force bool) error {
	absPath, err := filepath.Abs(outPath)
	if err != nil {
		return err
	}
	if isWithinRoot(root, absPath) {
		return errors.New("refusing to write plaintext inside the vault repository")
	}
	tracked := false
	if !allowGit && a.Sync.Git != nil {
		repoRoot, err := a.Sync.Git.TopLevel(ctx, filepath.Dir(absPath))
		if err == nil {
			isTracked, err := a.Sync.Git.IsPathTracked(ctx, repoRoot, absPath)
			if err == nil {
				tracked = isTracked
			}
		}
	}
	exists := false
	if !force {
		if _, err := os.Stat(absPath); err == nil {
			exists = true
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if tracked && !allowGit {
		if exists && !force {
			return errors.New("output file exists and is git-tracked; use --force and --allow-git to override")
		}
		return errors.New("refusing to write into git-tracked path without --allow-git")
	}
	if !force && exists {
		return errors.New("output file exists; use --force to overwrite")
	}
	return nil
}

func (a App) guardUpdatePath(ctx context.Context, root, targetPath string, allowGit bool) error {
	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		return err
	}
	if isWithinRoot(root, absPath) {
		return errors.New("refusing to write plaintext inside the vault repository")
	}
	if allowGit || a.Sync.Git == nil {
		return nil
	}
	repoRoot, err := a.Sync.Git.TopLevel(ctx, filepath.Dir(absPath))
	if err != nil {
		return nil
	}
	tracked, err := a.Sync.Git.IsPathTracked(ctx, repoRoot, absPath)
	if err != nil {
		return nil
	}
	if tracked {
		return errors.New("refusing to write into git-tracked path without --allow-git")
	}
	return nil
}

func writeEnvFile(path string, payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(payload)
	return err
}

func writeBinaryFile(path string, payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(payload)
	return err
}

func flattenEnv(values map[string]string) []string {
	pairs := make([]string, 0, len(values))
	for key, value := range values {
		pairs = append(pairs, key+"="+value)
	}
	return pairs
}

func isWithinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, "..")
}

func fillProjectEnv(project, env *string, args []string) ([]string, error) {
	if (*project == "") != (*env == "") {
		return args, errors.New("--project and --env must be provided together")
	}
	if *project == "" && *env == "" && len(args) >= 2 {
		*project = args[0]
		*env = args[1]
		return args[2:], nil
	}
	return args, nil
}

func splitKeyRef(ref string) (string, string, string) {
	parts := strings.SplitN(ref, "/", 3)
	if len(parts) == 3 {
		return parts[0], parts[1], parts[2]
	}
	return "", "", ref
}
