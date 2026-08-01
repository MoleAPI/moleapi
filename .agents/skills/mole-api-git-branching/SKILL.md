---
name: mole-api-git-branching
description: Git branch naming rules for the mole-api repository. Use before creating, switching, renaming, pushing, or planning local branches in mole-api, especially for test branches, version branches, release branches, prerelease branches, or when the user mentions “分支”, “版本分支”, “测试分支”, or “开分支”.
---

# Mole-API Git Branching

## Rules

- Do not create `codex/...` branches in this repository unless the user explicitly asks for that prefix.
- Do not create `version/<text>` branches for mole-api version work.
- For test-version work, use `prerelease/moleapi-<version>-devN`.
- For final release work, use `release/moleapi-<version>`.
- If the user asks for a new test version branch and no exact version is provided, infer the next patch version from the latest `release/moleapi-*` branch and use `dev1` unless a matching prerelease already exists.
- If an existing Codex-created or `version/<text>` branch is still local and unpushed, rename it to the matching mole-api version branch before continuing.

## Examples

- Payment button test after `release/moleapi-0.10.6.5`: `prerelease/moleapi-0.10.6.6-dev1`
- Version test rollout: `prerelease/moleapi-0.10.6.6-dev1`
- Final release branch: `release/moleapi-0.9.9`
