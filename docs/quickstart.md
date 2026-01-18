# GitVault Quickstart

## 1. Install prerequisites

- Install `sops` and `age` for your platform.
- Create an age identity (if you do not have one already):

```bash
age-keygen -o ~/.config/sops/age/keys.txt
```

If you store the identity elsewhere, set `SOPS_AGE_KEY_FILE=/path/to/keys.txt`.

## 2. Initialize a vault

```bash
gitvault init --path ./vault --name my-vault --recipient age1example...
```

This creates `.gitvault/` metadata and a `secrets/` directory in the vault. You can also omit
`--recipient` and add one later with `gitvault keys add`.

## 3. Add recipients (optional)

```bash
gitvault --vault ./vault keys add age1another...
```

If you already passed `--recipient` during init, you can skip this step until you need more recipients.

## 4. Set and list secrets

```bash
gitvault --vault ./vault secret set myapp dev API_KEY "abc123"
gitvault --vault ./vault secret list --project myapp --env dev
```

## 5. Import and export

```bash
gitvault --vault ./vault secret import-env --project myapp --env dev --file .env

gitvault --vault ./vault secret export-env --project myapp --env dev --out .env --force --allow-git
gitvault --vault ./vault secret export-env myapp dev --out .env --force --allow-git
```

Note: `--allow-git` is only required for git-tracked files. Untracked files inside a git repo are allowed.

## 6. Doctor

```bash
gitvault --vault ./vault doctor
```

## 7. Sync with git

```bash
gitvault --vault ./vault sync pull
gitvault --vault ./vault sync push
```
