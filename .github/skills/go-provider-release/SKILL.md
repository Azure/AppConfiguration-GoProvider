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

## Cycle: Create → Monitor Task → Monitor PR

Every PR in this release follows the same three-phase cycle. The steps below reference this cycle.

### A. Create the agent task

Create one agent task for the next PR in the sequence. Each command returns a URL in the format:

`https://github.com/Azure/AppConfiguration-GoProvider/pull/<pr-id>/agent-sessions/<agent-session-id>`

### B. Monitor the agent task

Extract the `agent-session-id` from the URL returned in step A, then poll:

```bash
gh agent-task view <agent-session-id>
```

Keep polling until the session state is `Ready for review`. Print the PR URL for reference.

### C. Monitor the PR until merged

Poll the PR every 10 minutes. Stop monitoring if:

- PR state is `MERGED` → proceed to the next step (or finish if this was the last PR).
- PR state is `CLOSED` or `Abandoned` → report and **stop the release**.
- 24 hours elapsed → report current status and **stop the release**.

```bash
gh pr view <pr-id> --repo Azure/AppConfiguration-GoProvider --json state --jq '.state'
```

---

## Procedure

Follow these steps **in order**. Each step depends on the previous one completing successfully.

### Step 1 — Create Version Bump PR

Use the **Create → Monitor Task → Monitor PR** cycle:

**A. Create the agent task:**

```bash
gh agent-task create \
  "Please create a version bump PR for version <version>. \
  Checkout a new branch from release/v<version> (e.g. version-bump/v<version>), \
  update the moduleVersion in azureappconfiguration/version.go to <version>, \
  and open a PR targeting the release/v<version> branch with title 'Version bump v<version>'." \
  --repo Azure/AppConfiguration-GoProvider \
  --base release/v<version>
```

**B. Monitor the agent task** until session state is `Ready for review`.

**C. Monitor the PR** until it is merged. Do not proceed until the PR state is `MERGED`.

### Step 2 — Tag the Release

After the version bump PR is merged, fetch the release branch and create a git tag at the HEAD:

```bash
git fetch origin release/v<version>
git tag azureappconfiguration/v<version> origin/release/v<version>
```

### Step 3 — Push the Tag

Push the tag to the remote:

```bash
git push origin azureappconfiguration/v<version>
```

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

```bash
GOPROXY=proxy.golang.org go list -m github.com/Azure/AppConfiguration-GoProvider/azureappconfiguration@v<version>
```

### Step 5 — Create Merge-Back PR

Use the **Create → Monitor Task → Monitor PR** cycle:

**A. Create the agent task:**

```bash
gh agent-task create \
  "Please create a PR to merge release/v<version> back to main with title 'Merge release/v<version> to main'." \
  --repo Azure/AppConfiguration-GoProvider \
  --base main
```

**B. Monitor the agent task** until session state is `Ready for review`.

**C. Monitor the PR** until it is merged. The release is complete once this PR is merged.

## Notes

- The version in `azureappconfiguration/version.go` uses the format `X.Y.Z` (no `v` prefix) in the `moduleVersion` constant.
- Tags use the format `azureappconfiguration/vX.Y.Z` (with `v` prefix and module path prefix).
- The publish command only changes the version portion: `@vX.Y.Z`.
