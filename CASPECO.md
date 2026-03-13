# CASPECO Fork Changes

Baseline commit: `96f625370ce6eb1c2f658646492681c14f59cbfe`

This document lists each change introduced in this fork after the baseline commit, one section per commit.

## 2025-12-18 - Configure build pipelines

### What changed
- Reduced and simplified GitHub Actions workflow surface.
- Removed many ASF/community maintenance workflows from this fork.
- Updated image build workflow behavior.

### Key details
- Deleted multiple workflows, including lint/check/stale/e2e helper workflows (for example: `NOTICE-year-check`, `asf-header-check`, `auto-cherry-pick`, `test-e2e`, `yaml-lint`, etc.).
- `build.yml` updates:
  - Added explicit `registry: ${{ secrets.DOCKERHUB_OWNER }}` to Docker login steps.
  - Limited one matrix build path to `amd64`.
  - Added a `Free Disk Space` step before build cache/build execution.
  - Removed the `build-and-push-other-image` job (`config-ui`, `grafana`).
- `test.yml` update:
  - Removed nightly scheduled run (`cron`).

---

## 2026-01-09 - Applied Caspeco Patch

### What changed
- Extended GitHub deployment model with explicit latest successful status timestamp.
- Updated GitHub GraphQL deployment collection/extraction/conversion behavior.

### Key details
- Added `latest_success_updated_date` to `_tool_github_deployments` model.
- Added migration script to create the new column:
  - `20251217_add_latest_status_update_date_to_deployment.go`
- GraphQL collector now requests deployment status history (`statuses(first: 10)`).
- Extractor now derives `LatestSuccessUpdatedDate`:
  - defaults to `LatestStatus.updatedAt`
  - overridden by first `SUCCESS` status timestamp if present.
- Deployment converter now uses `LatestSuccessUpdatedDate` as `finished_date` in domain conversion.
- Production classification tightened:
  - `ENV_NAME_PATTERN` match now also requires a version-like `ref_name` (`^\d+\.\d+`).
- Added `caspeco.patch` artifact file to the repo.

---

## 2026-02-13 - Deployment exclusion

### What changed
- Added post-extract SQL deduplication for GitHub GraphQL deployments.
- Updated build/runtime packaging and local orchestration assets.
- Adjusted Grafana DORA Change Failure Rate dashboard deployment filter.

### Key details
- Added `DedupDeployments` subtask in GitHub GraphQL:
  - Dedup key: `connection_id, github_id, environment, ref_name, commit_oid`
  - Keeps latest row by `COALESCE(latest_updated_date, updated_date)` (then `id DESC`)
  - Runs before deployment conversion.
- Wired dedup into subtask flow and dependency chain:
  - collect -> extract -> dedup -> convert
- `backend/Dockerfile` updates include:
  - zlib static PIC build from source and linkage adjustments
  - Poetry installer version pin (`2.2.1`)
  - base/build stage cleanup and dependency package adjustments
- Added root `docker-compose.yml` with `mysql`, `grafana`, `devlake`, and `config-ui` services.
- `grafana/Dockerfile` now sets `GF_PLUGINS_PREINSTALL_DISABLED=true`.
- `grafana/dashboards/DORADetails-ChangeFailureRate.json` now explicitly filters deployments to `environment in ('PRODUCTION')` in one query block.

---

## 2026-03-09 - Support cherry-pick-based deployments

### What changed
- Added DORA support for deployments that promote cherry-picked commits instead of the original PR merge commit.
- Added a per-repository GitHub scope-config flag and UI toggle to enable autodetection.
- Added focused unit tests for the new cherry-pick detector logic.

### Key details
- Added a new DORA subtask, `detectCherryPickedPullRequests`, before change lead time calculation.
- The detector scans commit diffs between consecutive successful production deployments and extracts PR references from commit messages using the `(#1234)` pattern.
- Detected matches are stored in a new `cicd_deployment_commit_pull_requests` table, keyed by project, deployment commit, PR, and matched commit SHA.
- Change lead time lookup now falls back to the autodetected deployment-to-PR mapping when a deployment cannot be linked through the PR merge commit directly.
- Added GitHub scope-config support for `autodetectCherryPickedPrs`:
  - backend model field
  - migration for `_tool_github_scope_configs`
  - default config value and Config UI checkbox under Additional Settings

---

## 2026-03-12 - Add GitHub webhook export flow and blueprint integration

### What changed
- Added a GitHub webhook export feature that can collect PRs, PR commits, PR comments, and deployments from GitHub and submit them to webhook connections.
- Added saved webhook export configurations on GitHub connections and blueprint-level export selection.
- Extended webhook ingestion to accept PR commits and PR comments directly.
- Integrated saved exports into blueprint execution ahead of DORA calculations.
- Added controls for excluding selected GitHub accounts from exported comments and reviews.

### Key details
- Added `POST /plugins/github/connections/:connectionId/webhook-export`:
  - supports repo-level export definitions
  - supports multiple team prefixes
  - supports exact-match workflow deployment sources and GitHub Deployments API deployment sources
  - supports retry logic and detailed progress logging
- Webhook plugin now exposes and accepts:
  - `pull_request_commits`
  - `pull_request_comments`
- Export submission now also writes explicit deployment-to-PR links with `detection_method = github_webhook_export_compare`.
- DORA behavior was extended to:
  - preserve exporter-created deployment/PR links during cherry-pick detection cleanup
  - prefer explicit webhook-export deployment mappings when computing lead time, but only for `github_webhook_export_compare`
- Added GitHub scope-config support for `convertGithubDeployment` to allow disabling standard GitHub deployment conversion when needed.
- Added `exclude_from_computation` to GitHub accounts so selected bot/tool accounts can be ignored during export of comments and reviews.
- Normal blueprint plans now derive webhook export jobs automatically from GitHub connection configuration by matching saved exports to the blueprint’s attached webhook connections, and inject a GitHub export stage before metric stages.
- Config UI updates include:
  - GitHub connection form support for multiple saved webhook exports
  - webhook connection dropdown selection
  - workflow-name list inputs
  - blueprint webhook export job visibility sourced from GitHub connection configuration
  - pipeline task naming for webhook export tasks
  - webhook “View Webhook” dialog support for PR commit/comment endpoints
  - webhook creation dialog support for PR commit/comment endpoints
- Workflow-based deployment export uses exact matches from `deploymentWorkflowNames`, while GitHub Deployments API export remains environment-pattern based for production-like names.
- Exported deployment display titles now use the newest matched PR title instead of a generic `deploy` label.
- Export compare logging is grouped per deployment and lists the matched PR numbers in one line.
- Removed the historical `caspeco-patches` patch artifact directory from the branch.
