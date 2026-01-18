# Team Key Usage

This guide describes how teams typically manage GitVault recipients (age public
keys) when the vault lives in a dedicated repository. It focuses on practical
workflows: onboarding, revocation, rotation, and what is possible or not.

## Model and roles

GitVault stores encrypted secrets and a plaintext index. Access is controlled
entirely by the recipient list in `.gitvault/config.json`. If a recipient is in
the list, they can decrypt all secrets in the vault.

Recommended roles:

- CI key: one shared recipient for automated jobs (stored in CI secret store).
- Per-developer key: one unique recipient per developer (private key stays local).
- Optional break-glass key: stored offline for recovery.

## Recommended setup

Keep the vault in its own repo, separate from app repos. Developers and CI
interact with it via `gitvault --vault <path> ...`.

Onboarding a developer:

```bash
age-keygen -o ~/.config/sops/age/keys.txt
RECIPIENT=$(age-keygen -y ~/.config/sops/age/keys.txt)
gitvault --vault ./vault keys add "$RECIPIENT"
gitvault --vault ./vault keys list
```

Adding a CI key:

```bash
CI_RECIPIENT="age1..."
gitvault --vault ./vault keys add "$CI_RECIPIENT"
```

Store the CI private key in a secure secret manager and expose it to jobs via
`SOPS_AGE_KEY_FILE`.

## Revoking access

If a developer leaves, remove their recipient and re-encrypt:

```bash
gitvault --vault ./vault keys remove age1former...
gitvault --vault ./vault keys rotate
```

`keys rotate` re-encrypts existing secret files using the current recipients
list. It does not change secret values.

## Compromised key procedure

If a key is compromised, you should both remove the recipient and rotate the
secret values in downstream systems.

```bash
gitvault --vault ./vault keys remove age1compromised...
gitvault --vault ./vault keys rotate
```

Then rotate the actual secrets (API keys, tokens, passwords) at their source
and update the vault with new values.

## Key rotation without incident

For routine rotation, add the new key first, re-encrypt, then remove the old:

```bash
gitvault --vault ./vault keys add age1new...
gitvault --vault ./vault keys rotate
gitvault --vault ./vault keys remove age1old...
```

## What is possible (and not)

Possible:

- Multiple recipients per vault (devs + CI).
- Revocation by removing a recipient and re-encrypting secrets.
- Rotation by re-encrypting secrets with a new recipient set.

Not possible with a single vault:

- Per-secret access control. Any recipient can decrypt all secrets.

If you need different access scopes, use multiple vaults (for example, split by
project or environment) or separate vault repositories per team.

## Practical notes

- Never share private age keys. Only public recipients go into the vault.
- Use `gitvault doctor` to verify key availability locally.
- Track recipient changes in git history for auditability.
