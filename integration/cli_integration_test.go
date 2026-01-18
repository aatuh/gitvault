package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aatuh/gitvault/internal/testutil"
)

var (
	gitvaultBin string
	sopsBin     string
	ageKeyFile  string
	repoRoot    string
	useRealSops = flag.Bool("real-sops", false, "use real sops binary instead of stub")
	sopsPath    = flag.String("sops-path", "sops", "path to sops binary when -real-sops is set")
	sopsKeyFile = flag.String("sops-age-key-file", "", "path to age key file when -real-sops is set")
	sopsRecip   = flag.String("sops-recipient", "", "age recipient to use when -real-sops is set")
)

func TestMain(m *testing.M) {
	flag.Parse()
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cwd:", err)
		os.Exit(1)
	}
	repoRoot = filepath.Clean(filepath.Join(cwd, ".."))
	tmpDir, err := os.MkdirTemp("", "gitvault-integration-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "tempdir:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	gitvaultBin = filepath.Join(tmpDir, "gitvault")
	sopsBin = filepath.Join(tmpDir, "sops")
	if runtime.GOOS == "windows" {
		gitvaultBin += ".exe"
		if !*useRealSops {
			sopsBin += ".exe"
		}
	}

	if err := buildBinary(repoRoot, gitvaultBin, "./cmd/gitvault"); err != nil {
		fmt.Fprintln(os.Stderr, "build gitvault:", err)
		os.Exit(1)
	}
	if *useRealSops {
		if strings.TrimSpace(*sopsPath) == "" {
			fmt.Fprintln(os.Stderr, "sops-path cannot be empty when -real-sops is set")
			os.Exit(1)
		}
		path, err := exec.LookPath(*sopsPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sops not found:", err)
			os.Exit(1)
		}
		sopsBin = path
		if strings.TrimSpace(*sopsKeyFile) != "" {
			ageKeyFile = *sopsKeyFile
			if _, err := os.Stat(ageKeyFile); err != nil {
				fmt.Fprintln(os.Stderr, "age key file not found:", err)
				os.Exit(1)
			}
		}
	} else {
		stubPath := filepath.Join(tmpDir, "sops_stub.go")
		if err := os.WriteFile(stubPath, []byte(sopsStubSource), 0600); err != nil {
			fmt.Fprintln(os.Stderr, "write sops stub:", err)
			os.Exit(1)
		}
		if err := buildBinary(tmpDir, sopsBin, stubPath); err != nil {
			fmt.Fprintln(os.Stderr, "build sops stub:", err)
			os.Exit(1)
		}

		ageKeyFile = filepath.Join(tmpDir, "age-keys.txt")
		if err := os.WriteFile(ageKeyFile, []byte("AGE-SECRET-KEY-1TEST\n"), 0600); err != nil {
			fmt.Fprintln(os.Stderr, "write age key:", err)
			os.Exit(1)
		}
	}

	os.Exit(m.Run())
}

func buildBinary(dir, output, target string) error {
	cmd := exec.Command("go", "build", "-o", output, target)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go build: %w: %s", err, strings.TrimSpace(string(outputBytes)))
	}
	return nil
}

type commandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func runGitvault(t *testing.T, env map[string]string, args ...string) commandResult {
	t.Helper()
	cmd := exec.Command(gitvaultBin, args...)
	cmd.Env = append(os.Environ(), "GITVAULT_SOPS_PATH="+sopsBin)
	if ageKeyFile != "" {
		cmd.Env = append(cmd.Env, "SOPS_AGE_KEY_FILE="+ageKeyFile)
	}
	if stdinValue, ok := env["GITVAULT_TEST_STDIN"]; ok {
		cmd.Stdin = strings.NewReader(stdinValue)
	}
	for key, value := range env {
		if key == "GITVAULT_TEST_STDIN" {
			continue
		}
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := commandResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: 0}
	if err == nil {
		return result
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result
	}
	result.ExitCode = 1
	return result
}

func TestInitAndDoctor(t *testing.T) {
	vaultDir := t.TempDir()
	recipient := testRecipient(t)
	name := "vault-" + testutil.RandomString(t, 6)

	result := runGitvault(t, nil, "init", "--path", vaultDir, "--name", name, "--recipient", recipient, "--skip-git")
	if result.ExitCode != 0 {
		t.Fatalf("init failed: %s", result.Stderr)
	}
	if !strings.Contains(result.Stdout, "vault initialized") {
		t.Fatalf("expected init confirmation, got: %s", result.Stdout)
	}

	doctor := runGitvault(t, nil, "--vault", vaultDir, "doctor")
	if doctor.ExitCode != 0 {
		t.Fatalf("doctor failed: %s", doctor.Stderr)
	}
	if !strings.Contains(doctor.Stdout, "vault config") || !strings.Contains(doctor.Stdout, "sops") {
		t.Fatalf("doctor output missing checks: %s", doctor.Stdout)
	}
}

func TestHelpOutputs(t *testing.T) {
	secretHelp := runGitvault(t, nil, "secret", "--help")
	if secretHelp.ExitCode != 0 {
		t.Fatalf("secret help failed: %s", secretHelp.Stderr)
	}
	if !strings.Contains(secretHelp.Stdout, "import-env") || !strings.Contains(secretHelp.Stdout, "export-env") {
		t.Fatalf("secret help missing subcommands")
	}

	secretSetHelp := runGitvault(t, nil, "secret", "set", "--help")
	if secretSetHelp.ExitCode != 0 {
		t.Fatalf("secret set help failed: %s", secretSetHelp.Stderr)
	}
	if !strings.Contains(secretSetHelp.Stdout, "secret set") {
		t.Fatalf("secret set usage missing")
	}

	envHelp := runGitvault(t, nil, "env", "--help")
	if envHelp.ExitCode != 0 {
		t.Fatalf("env help failed: %s", envHelp.Stderr)
	}
	if !strings.Contains(envHelp.Stdout, "env list") {
		t.Fatalf("env help missing usage")
	}

	projectHelp := runGitvault(t, nil, "project", "--help")
	if projectHelp.ExitCode != 0 {
		t.Fatalf("project help failed: %s", projectHelp.Stderr)
	}
	if !strings.Contains(projectHelp.Stdout, "project list") {
		t.Fatalf("project help missing usage")
	}

	keysHelp := runGitvault(t, nil, "keys", "--help")
	if keysHelp.ExitCode != 0 {
		t.Fatalf("keys help failed: %s", keysHelp.Stderr)
	}
	if !strings.Contains(keysHelp.Stdout, "keys add") {
		t.Fatalf("keys help missing usage")
	}

	syncHelp := runGitvault(t, nil, "sync", "--help")
	if syncHelp.ExitCode != 0 {
		t.Fatalf("sync help failed: %s", syncHelp.Stderr)
	}
	if !strings.Contains(syncHelp.Stdout, "sync pull") {
		t.Fatalf("sync help missing usage")
	}
}

func TestKeysLifecycle(t *testing.T) {
	vaultDir := t.TempDir()
	recipient := testRecipient(t)

	result := runGitvault(t, nil, "init", "--path", vaultDir, "--name", "vault", "--recipient", recipient, "--skip-git")
	if result.ExitCode != 0 {
		t.Fatalf("init failed: %s", result.Stderr)
	}

	list := runGitvault(t, nil, "--vault", vaultDir, "keys", "list")
	if list.ExitCode != 0 {
		t.Fatalf("keys list failed: %s", list.Stderr)
	}
	if !strings.Contains(list.Stdout, recipient) {
		t.Fatalf("expected recipient in list")
	}

	newRecipient := "age1" + testutil.RandomString(t, 10)
	add := runGitvault(t, nil, "--vault", vaultDir, "keys", "add", newRecipient)
	if add.ExitCode != 0 {
		t.Fatalf("keys add failed: %s", add.Stderr)
	}

	list = runGitvault(t, nil, "--vault", vaultDir, "keys", "list")
	if !strings.Contains(list.Stdout, newRecipient) {
		t.Fatalf("expected new recipient in list")
	}

	remove := runGitvault(t, nil, "--vault", vaultDir, "keys", "remove", recipient)
	if remove.ExitCode != 0 {
		t.Fatalf("keys remove failed: %s", remove.Stderr)
	}
	list = runGitvault(t, nil, "--vault", vaultDir, "keys", "list")
	if strings.Contains(list.Stdout, recipient) {
		t.Fatalf("expected removed recipient gone")
	}
}

func TestSecretWorkflowAndListing(t *testing.T) {
	vaultDir := t.TempDir()
	recipient := testRecipient(t)
	project := randomIdentifier(t)
	envName := randomIdentifier(t)

	result := runGitvault(t, nil, "init", "--path", vaultDir, "--name", "vault", "--recipient", recipient, "--skip-git")
	if result.ExitCode != 0 {
		t.Fatalf("init failed: %s", result.Stderr)
	}

	key1 := "API_KEY"
	value1 := testutil.RandomString(t, 12)
	set := runGitvault(t, nil, "--vault", vaultDir, "secret", "set", project, envName, key1, value1)
	if set.ExitCode != 0 {
		t.Fatalf("secret set failed: %s", set.Stderr)
	}

	key2 := "TOKEN"
	value2 := testutil.RandomString(t, 10)
	stdinResult := runGitvault(t, map[string]string{"GITVAULT_TEST_STDIN": value2}, "--vault", vaultDir, "secret", "set", "--stdin", project, envName, key2)
	if stdinResult.ExitCode != 0 {
		t.Fatalf("secret set stdin failed: %s", stdinResult.Stderr)
	}

	key3 := "FLAG_KEY"
	value3 := testutil.RandomString(t, 9)
	setFlags := runGitvault(t, nil, "--vault", vaultDir, "secret", "set", "--project", project, "--env", envName, key3, value3)
	if setFlags.ExitCode != 0 {
		t.Fatalf("secret set (flags) failed: %s", setFlags.Stderr)
	}

	list := runGitvault(t, nil, "--vault", vaultDir, "secret", "list", project, envName, "--show-last-changed")
	if list.ExitCode != 0 {
		t.Fatalf("secret list failed: %s", list.Stderr)
	}
	if !strings.Contains(list.Stdout, key1) || !strings.Contains(list.Stdout, key2) || !strings.Contains(list.Stdout, key3) {
		t.Fatalf("secret list missing keys: %s", list.Stdout)
	}
	if !strings.Contains(list.Stdout, "last_updated") {
		t.Fatalf("expected last_updated header")
	}

	allList := runGitvault(t, nil, "--vault", vaultDir, "secret", "list", "--show-last-changed")
	if allList.ExitCode != 0 {
		t.Fatalf("secret list all failed: %s", allList.Stderr)
	}
	if !strings.Contains(allList.Stdout, "project") || !strings.Contains(allList.Stdout, "env") || !strings.Contains(allList.Stdout, "key") {
		t.Fatalf("expected project/env/key headers")
	}
	if !strings.Contains(allList.Stdout, project) || !strings.Contains(allList.Stdout, envName) || !strings.Contains(allList.Stdout, key1) {
		t.Fatalf("expected entries in list all output")
	}

	find := runGitvault(t, nil, "--vault", vaultDir, "secret", "find", project)
	if find.ExitCode != 0 {
		t.Fatalf("secret find failed: %s", find.Stderr)
	}
	if !strings.Contains(find.Stdout, project+"/"+envName+"/"+key1) {
		t.Fatalf("secret find missing reference")
	}

	projectList := runGitvault(t, nil, "--vault", vaultDir, "project", "list")
	if projectList.ExitCode != 0 {
		t.Fatalf("project list failed: %s", projectList.Stderr)
	}
	if !strings.Contains(projectList.Stdout, project) {
		t.Fatalf("project list missing project")
	}

	envList := runGitvault(t, nil, "--vault", vaultDir, "env", "list", "--project", project)
	if envList.ExitCode != 0 {
		t.Fatalf("env list failed: %s", envList.Stderr)
	}
	if !strings.Contains(envList.Stdout, envName) {
		t.Fatalf("env list missing env")
	}

	exportResult := runGitvault(t, nil, "--vault", vaultDir, "secret", "export", "--project", project, "--env", envName)
	if exportResult.ExitCode != 0 {
		t.Fatalf("export failed: %s", exportResult.Stderr)
	}
	if !strings.Contains(exportResult.Stdout, key1+"="+value1) || !strings.Contains(exportResult.Stdout, key2+"="+value2) {
		t.Fatalf("export output missing keys")
	}

	outPath := filepath.Join(t.TempDir(), ".env")
	exportFile := runGitvault(t, nil, "--vault", vaultDir, "secret", "export-env", project, envName, "--out", outPath, "--force")
	if exportFile.ExitCode != 0 {
		t.Fatalf("export file failed: %s", exportFile.Stderr)
	}
	exportedData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read export file: %v", err)
	}
	if !strings.Contains(string(exportedData), key3+"="+value3) {
		t.Fatalf("export file missing keys")
	}

	envFile := filepath.Join(t.TempDir(), ".env")
	content := []byte("NEW_KEY=" + testutil.RandomString(t, 8) + "\n")
	if err := os.WriteFile(envFile, content, 0600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	importResult := runGitvault(t, nil, "--vault", vaultDir, "secret", "import", "--project", project, "--env", envName, "--file", envFile, "--strategy", "prefer-file")
	if importResult.ExitCode != 0 {
		t.Fatalf("import failed: %s", importResult.Stderr)
	}
	if !strings.Contains(importResult.Stdout, "import complete") {
		t.Fatalf("expected import confirmation")
	}
	exportResult = runGitvault(t, nil, "--vault", vaultDir, "secret", "export-env", "--project", project, "--env", envName)
	if !strings.Contains(exportResult.Stdout, "NEW_KEY=") {
		t.Fatalf("expected imported key in export")
	}

	unset := runGitvault(t, nil, "--vault", vaultDir, "secret", "unset", project, envName, key1)
	if unset.ExitCode != 0 {
		t.Fatalf("unset failed: %s", unset.Stderr)
	}

	list = runGitvault(t, nil, "--vault", vaultDir, "secret", "list", "--project", project, "--env", envName)
	if strings.Contains(list.Stdout, key1) {
		t.Fatalf("expected key removed from list")
	}
}

func TestExportGuardrailsAndJSON(t *testing.T) {
	vaultDir := t.TempDir()
	recipient := testRecipient(t)
	project := randomIdentifier(t)
	envName := randomIdentifier(t)

	result := runGitvault(t, nil, "init", "--path", vaultDir, "--name", "vault", "--recipient", recipient, "--skip-git")
	if result.ExitCode != 0 {
		t.Fatalf("init failed: %s", result.Stderr)
	}

	key := "API_KEY"
	value := testutil.RandomString(t, 10)
	set := runGitvault(t, nil, "--vault", vaultDir, "secret", "set", project, envName, key, value)
	if set.ExitCode != 0 {
		t.Fatalf("secret set failed: %s", set.Stderr)
	}

	outPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(outPath, []byte("existing"), 0600); err != nil {
		t.Fatalf("write existing file: %v", err)
	}

	deny := runGitvault(t, nil, "--vault", vaultDir, "secret", "export-env", "--project", project, "--env", envName, "--out", outPath)
	if deny.ExitCode == 0 {
		t.Fatalf("expected export to fail without --force")
	}
	if !strings.Contains(deny.Stderr, "use --force") {
		t.Fatalf("expected force guidance")
	}

	allow := runGitvault(t, nil, "--vault", vaultDir, "secret", "export-env", "--project", project, "--env", envName, "--out", outPath, "--force")
	if allow.ExitCode != 0 {
		t.Fatalf("export with force failed: %s", allow.Stderr)
	}
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat output file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected 0600 perms, got %v", info.Mode().Perm())
	}

	insideVault := filepath.Join(vaultDir, "export.env")
	inside := runGitvault(t, nil, "--vault", vaultDir, "secret", "export-env", "--project", project, "--env", envName, "--out", insideVault, "--force")
	if inside.ExitCode == 0 {
		t.Fatalf("expected refusal to export inside vault")
	}
	if !strings.Contains(inside.Stderr, "vault repository") {
		t.Fatalf("expected vault repository warning")
	}

	jsonResult := runGitvault(t, nil, "--vault", vaultDir, "--json", "secret", "list", "--project", project, "--env", envName)
	if jsonResult.ExitCode != 0 {
		t.Fatalf("json list failed: %s", jsonResult.Stderr)
	}
	if !json.Valid([]byte(jsonResult.Stdout)) {
		t.Fatalf("expected valid json output")
	}
}

func TestKeysRotate(t *testing.T) {
	vaultDir := t.TempDir()
	recipient := testRecipient(t)
	project := randomIdentifier(t)
	envName := randomIdentifier(t)

	result := runGitvault(t, nil, "init", "--path", vaultDir, "--name", "vault", "--recipient", recipient, "--skip-git")
	if result.ExitCode != 0 {
		t.Fatalf("init failed: %s", result.Stderr)
	}

	key := "API_KEY"
	value := testutil.RandomString(t, 12)
	set := runGitvault(t, nil, "--vault", vaultDir, "secret", "set", project, envName, key, value)
	if set.ExitCode != 0 {
		t.Fatalf("secret set failed: %s", set.Stderr)
	}

	rotate := runGitvault(t, nil, "--vault", vaultDir, "--json", "keys", "rotate")
	if rotate.ExitCode != 0 {
		t.Fatalf("rotate failed: %s", rotate.Stderr)
	}
	var payload struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(rotate.Stdout), &payload); err != nil {
		t.Fatalf("parse rotate json: %v", err)
	}
	if payload.Data["rotated"] == nil {
		t.Fatalf("expected rotated count")
	}
}

func TestSecretRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires sh")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	vaultDir := t.TempDir()
	recipient := testRecipient(t)
	project := randomIdentifier(t)
	envName := randomIdentifier(t)

	result := runGitvault(t, nil, "init", "--path", vaultDir, "--name", "vault", "--recipient", recipient, "--skip-git")
	if result.ExitCode != 0 {
		t.Fatalf("init failed: %s", result.Stderr)
	}

	key := "API_KEY"
	value := testutil.RandomString(t, 12)
	set := runGitvault(t, nil, "--vault", vaultDir, "secret", "set", project, envName, key, value)
	if set.ExitCode != 0 {
		t.Fatalf("secret set failed: %s", set.Stderr)
	}

	cmd := runGitvault(t, nil, "--vault", vaultDir, "secret", "run", "--project", project, "--env", envName, "--", "sh", "-c", "echo -n $API_KEY")
	if cmd.ExitCode != 0 {
		t.Fatalf("secret run failed: %s", cmd.Stderr)
	}
	if strings.TrimSpace(cmd.Stdout) != value {
		t.Fatalf("expected injected env value, got %q", cmd.Stdout)
	}
}

func TestGitSync(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	vaultDir := t.TempDir()
	recipient := testRecipient(t)

	init := runGitvault(t, nil, "init", "--path", vaultDir, "--name", "vault", "--recipient", recipient)
	if init.ExitCode != 0 {
		t.Fatalf("init failed: %s", init.Stderr)
	}

	commitEnv := gitEnv()
	if err := runGit(t, vaultDir, commitEnv, "add", "."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := runGit(t, vaultDir, commitEnv, "commit", "-m", "init"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	remoteDir := filepath.Join(t.TempDir(), "remote.git")
	if err := runGit(t, filepath.Dir(remoteDir), commitEnv, "init", "--bare", remoteDir); err != nil {
		t.Fatalf("git init --bare: %v", err)
	}
	if err := runGit(t, vaultDir, commitEnv, "remote", "add", "origin", remoteDir); err != nil {
		t.Fatalf("git remote add: %v", err)
	}

	if err := runGit(t, vaultDir, commitEnv, "push", "-u", "origin", "HEAD"); err != nil {
		t.Fatalf("git push -u: %v", err)
	}
	localFile := filepath.Join(vaultDir, "LOCAL.md")
	if err := os.WriteFile(localFile, []byte("local"), 0600); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	if err := runGit(t, vaultDir, commitEnv, "add", "LOCAL.md"); err != nil {
		t.Fatalf("git add local: %v", err)
	}
	if err := runGit(t, vaultDir, commitEnv, "commit", "-m", "local change"); err != nil {
		t.Fatalf("git commit local: %v", err)
	}

	push := runGitvault(t, nil, "--vault", vaultDir, "sync", "push")
	if push.ExitCode != 0 {
		t.Fatalf("sync push failed: %s", push.Stderr)
	}

	cloneDir := filepath.Join(t.TempDir(), "clone")
	if err := runGit(t, filepath.Dir(cloneDir), commitEnv, "clone", remoteDir, cloneDir); err != nil {
		t.Fatalf("git clone: %v", err)
	}
	newFile := filepath.Join(cloneDir, "REMOTE.md")
	if err := os.WriteFile(newFile, []byte("remote"), 0600); err != nil {
		t.Fatalf("write remote file: %v", err)
	}
	if err := runGit(t, cloneDir, commitEnv, "add", "REMOTE.md"); err != nil {
		t.Fatalf("git add remote: %v", err)
	}
	if err := runGit(t, cloneDir, commitEnv, "commit", "-m", "remote change"); err != nil {
		t.Fatalf("git commit remote: %v", err)
	}
	if err := runGit(t, cloneDir, commitEnv, "push", "origin", "HEAD"); err != nil {
		t.Fatalf("git push remote: %v", err)
	}

	pull := runGitvault(t, nil, "--vault", vaultDir, "sync", "pull")
	if pull.ExitCode != 0 {
		t.Fatalf("sync pull failed: %s", pull.Stderr)
	}
	if _, err := os.Stat(filepath.Join(vaultDir, "REMOTE.md")); err != nil {
		t.Fatalf("expected pulled file: %v", err)
	}
}

func TestExportGuardrailsGitTracked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	vaultDir := t.TempDir()
	recipient := testRecipient(t)
	project := randomIdentifier(t)
	envName := randomIdentifier(t)

	result := runGitvault(t, nil, "init", "--path", vaultDir, "--name", "vault", "--recipient", recipient, "--skip-git")
	if result.ExitCode != 0 {
		t.Fatalf("init failed: %s", result.Stderr)
	}

	key := "API_KEY"
	value := testutil.RandomString(t, 10)
	set := runGitvault(t, nil, "--vault", vaultDir, "secret", "set", project, envName, key, value)
	if set.ExitCode != 0 {
		t.Fatalf("secret set failed: %s", set.Stderr)
	}

	repoDir := t.TempDir()
	commitEnv := gitEnv()
	if err := runGit(t, repoDir, commitEnv, "init"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	outputPath := filepath.Join(repoDir, ".env")
	if err := os.WriteFile(outputPath, []byte("placeholder"), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := runGit(t, repoDir, commitEnv, "add", ".env"); err != nil {
		t.Fatalf("git add: %v", err)
	}

	deny := runGitvault(t, nil, "--vault", vaultDir, "secret", "export-env", "--project", project, "--env", envName, "--out", outputPath, "--force")
	if deny.ExitCode == 0 {
		t.Fatalf("expected export to fail for git-tracked path")
	}
	if !strings.Contains(deny.Stderr, "--allow-git") {
		t.Fatalf("expected allow-git hint")
	}

	allow := runGitvault(t, nil, "--vault", vaultDir, "secret", "export-env", "--project", project, "--env", envName, "--out", outputPath, "--force", "--allow-git")
	if allow.ExitCode != 0 {
		t.Fatalf("export with allow-git failed: %s", allow.Stderr)
	}
}

func gitEnv() []string {
	base := os.Environ()
	base = append(base,
		"GIT_AUTHOR_NAME=GitVault",
		"GIT_AUTHOR_EMAIL=gitvault@example.com",
		"GIT_COMMITTER_NAME=GitVault",
		"GIT_COMMITTER_EMAIL=gitvault@example.com",
	)
	return base
}

func runGit(t *testing.T, dir string, env []string, args ...string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func randomIdentifier(t *testing.T) string {
	t.Helper()
	value := testutil.RandomString(t, 6)
	value = strings.ReplaceAll(value, "-", "")
	value = strings.ReplaceAll(value, "_", "")
	return "p" + value
}

func testRecipient(t *testing.T) string {
	t.Helper()
	if !*useRealSops {
		return "age1" + testutil.RandomString(t, 8)
	}
	if strings.TrimSpace(*sopsRecip) != "" {
		return *sopsRecip
	}
	path := strings.TrimSpace(*sopsKeyFile)
	if path == "" {
		path = strings.TrimSpace(os.Getenv("SOPS_AGE_KEY_FILE"))
	}
	if path == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, ".config", "sops", "age", "keys.txt")
		}
	}
	if path == "" {
		t.Fatalf("real-sops requires -sops-recipient or a readable age key file")
	}
	recipient, err := readAgeRecipient(path)
	if err != nil {
		t.Fatalf("resolve recipient: %v", err)
	}
	return recipient
}

func readAgeRecipient(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			continue
		}
		lower := strings.ToLower(line)
		const marker = "public key:"
		idx := strings.Index(lower, marker)
		if idx == -1 {
			continue
		}
		return strings.TrimSpace(line[idx+len(marker):]), nil
	}
	return "", fmt.Errorf("no public key in %s; set -sops-recipient", path)
}

const sopsStubSource = `package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "missing args")
		os.Exit(2)
	}
	for _, arg := range os.Args[1:] {
		if arg == "--version" {
			fmt.Println("sops 0.0.0-test")
			return
		}
	}
	mode := ""
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--encrypt":
			mode = "encrypt"
		case "--decrypt":
			mode = "decrypt"
		}
	}
	file := os.Args[len(os.Args)-1]
	data, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	switch mode {
	case "encrypt":
		encoded := base64.StdEncoding.EncodeToString(data)
		fmt.Printf("ENC:%s", encoded)
	case "decrypt":
		text := string(data)
		if !strings.HasPrefix(text, "ENC:") {
			fmt.Fprintln(os.Stderr, "invalid ciphertext")
			os.Exit(1)
		}
		payload := strings.TrimPrefix(text, "ENC:")
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Print(string(decoded))
	default:
		fmt.Fprintln(os.Stderr, "unsupported args")
		os.Exit(2)
	}
}
`
