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

- You must be on the repository: `AppConfiguration-GoProvider`
- You need appropriate permissions to push tags and create PRs

## Procedure

Follow these steps **in order**. Each step depends on the previous one completing successfully.

### Step 1 — Create Version Bump PR

Use the GitHub agent to create a pull request:
- **Source branch**: `release/v<version>` (e.g. `release/v1.1.0`)
- **Target branch**: `main`
- **Change**: Update `moduleVersion` in `azureappconfiguration/version.go` to the new version
- **PR title**: `Release v<version>`

Wait for the PR to be created, then inform the user to review and merge it.

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

Use the GitHub agent to create a pull request to merge the release branch back to main:
- **Source branch**: `release/v<version>`
- **Target branch**: `main`
- **PR title**: `Merge release/v<version> back to main`

## Notes

- The version in `azureappconfiguration/version.go` uses the format `X.Y.Z` (no `v` prefix) in the `moduleVersion` constant.
- Tags use the format `azureappconfiguration/vX.Y.Z` (with `v` prefix and module path prefix).
- The publish command only changes the version portion: `@vX.Y.Z`.
