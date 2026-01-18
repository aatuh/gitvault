# GitVault

GitVault is a git-backed secret manager (Go CLI) that stores encrypted secrets
in a dedicated vault repository. It uses SOPS to encrypt dotenv files with age
recipients and keeps a plaintext index for fast listing without decrypting.

## Quickstart

Prereqs:

- Install `sops` and `age`.
- Ensure your age identity is available (default: `~/.config/sops/age/keys.txt`).
  If you store it elsewhere (e.g., `./keys.txt`), set `SOPS_AGE_KEY_FILE`.

First secret (copy/paste minimal flow):

```bash
# If your age identity lives elsewhere, set SOPS_AGE_KEY_FILE first.
# export SOPS_AGE_KEY_FILE=./keys.txt

age-keygen -o ~/.config/sops/age/keys.txt
RECIPIENT=$(age-keygen -y ~/.config/sops/age/keys.txt)

gitvault init --path ./vault --name my-vault --recipient "$RECIPIENT"
gitvault --vault ./vault secret set myapp dev API_KEY "abc123"
gitvault --vault ./vault secret export-env myapp dev --out .env --force --allow-git

# Optional sanity checks:
gitvault --vault ./vault secret list myapp dev
gitvault --vault ./vault doctor
```

Tip: `gitvault init --recipient` is the fastest path; you can also add recipients later with `gitvault keys add`.

Initialize a vault:

```bash
gitvault init --path ./vault --name my-vault --recipient age1example...
```

Add recipients later:

```bash
gitvault --vault ./vault keys add age1another...
```

Set secrets:

```bash
gitvault --vault ./vault secret set myapp dev API_KEY "abc123"
```

Import from a local `.env`:

```bash
gitvault --vault ./vault secret import-env --project myapp --env dev --file .env
```

Export to stdout or a file:

```bash
gitvault --vault ./vault secret export-env --project myapp --env dev
gitvault --vault ./vault secret export-env --project myapp --env dev --out .env --force --allow-git
```

List keys without decrypting values:

```bash
gitvault --vault ./vault secret list --project myapp --env dev --show-last-changed
```

Run a command with secrets injected (no `.env` on disk):

```bash
gitvault --vault ./vault secret run --project myapp --env dev -- ./run-server
```

Health check:

```bash
gitvault --vault ./vault doctor
```

## Vault Layout

- `.gitvault/config.json`: vault config (recipients, version)
- `.gitvault/index.json`: plaintext index (projects/envs/keys + last updated)
- `secrets/<project>/<env>.env`: encrypted SOPS dotenv files

## Safe Defaults

- Export refuses to overwrite existing files without `--force`.
- Export refuses to write into git-tracked paths without `--allow-git` (untracked files inside a repo are allowed).
- Export refuses to write plaintext inside the vault repo.

See `docs/guardrails.md` for recommended `.gitignore` and incident response.

## Output Format

Use `--json` for machine-readable output. Errors go to stderr and return a
non-zero exit code.

## Environment Variables

- `GITVAULT_SOPS_PATH`: override `sops` binary path.
- `SOPS_AGE_KEY_FILE`: override the age identity file.

## Development

```bash
make test
make build
```
