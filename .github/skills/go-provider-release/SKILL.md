---
name: go-provider-release
description: "Release the Azure App Configuration Go Provider. Use when: releasing a new version, bumping version, tagging a release, publishing to Go proxy, creating release PRs."
argument-hint: "Target version number, e.g. 1.1.0"
---

# Go Provider Release

## When to Use

- Release a new version of the Azure App Configuration Go Provider
- Bump the module version and publish to the Go module proxy

## Prerequisites

- GitHub CLI (`gh`) ≥ 2.86.0
- Authenticated via `gh auth login` with `repo` and `read:org` scopes
- GitHub Copilot agent enabled for the repository
- Write access to `Azure/AppConfiguration-GoProvider`

## Procedure

Follow these steps **in order**. Each step depends on the previous one completing successfully.

### Step 1 — Create Version Bump PR

Use `gh agent-task create` to create a version bump PR:

```bash
gh agent-task create \
  "Please create a version bump PR for version <version>. \
  Checkout a new branch from release/v<version> (e.g. version-bump/v<version>), \
  update the moduleVersion in azureappconfiguration/version.go to <version>, \
  and open a PR targeting the release/v<version> branch with title 'Version bump v<version>'." \
  --repo Azure/AppConfiguration-GoProvider \
  --base release/v<version>
```

After launching the agent task, monitor its progress:

```bash
gh agent-task list --repo Azure/AppConfiguration-GoProvider
```

Once the agent task completes and the PR is created, inform the user to review and merge it.

> **Pause here.** Do not proceed until the user confirms the version bump PR has been merged.

### Step 2 — Tag the Release

After the version bump PR is merged, create a git tag at the HEAD of the release branch:

```
git tag azureappconfiguration/v<version>
```

Example: `git tag azureappconfiguration/v1.1.0`

### Step 3 — Push the Tag

Push the tag to the remote:

```
git push origin azureappconfiguration/v<version>
```

Example: `git push origin azureappconfiguration/v1.1.0`

### Step 4 — Publish to Go Module Proxy

**Before executing the publish command**, generate a summary report table for human review:

| Item                | Detail                                                              |
|---------------------|---------------------------------------------------------------------|
| **Version**         | `v<version>`                                                        |
| **Version file**    | `azureappconfiguration/version.go` updated to `<version>`          |
| **Tag pushed**      | `azureappconfiguration/v<version>`                                  |
| **Publish command** | `GOPROXY=proxy.golang.org go list -m github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration@v<version>` |
| **Next step**       | After publish, create merge-back PR (release branch → main)        |

> **Pause here.** Present the table and wait for the user to confirm before proceeding.

After user confirmation, run:

```
GOPROXY=proxy.golang.org go list -m github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration@v<version>
```

### Step 5 — Create Merge-Back PR

Use `gh agent-task create` to create a pull request to merge the release branch back to main:

```bash
gh agent-task create \
  "Please create a PR to merge release/v<version> back to main with title 'Merge release/v<version> to main'." \
  --repo Azure/AppConfiguration-GoProvider \
  --base main
```

Monitor the agent task until the PR is created:

```bash
gh agent-task list --repo Azure/AppConfiguration-GoProvider
```

## Notes

- The version in `azureappconfiguration/version.go` uses the format `X.Y.Z` (no `v` prefix) in the `moduleVersion` constant.
- Tags use the format `azureappconfiguration/vX.Y.Z` (with `v` prefix and module path prefix).
- The publish command only changes the version portion: `@vX.Y.Z`.
