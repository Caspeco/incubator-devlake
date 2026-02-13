# CASPECO Fork Changes

Baseline commit: `96f625370ce6eb1c2f658646492681c14f59cbfe`

This document lists each change introduced in this fork after the baseline commit, one section per commit.

## 2025-12-18 - Configure build pipelines (`bdb549197716fa6ce663372f11268d22865afd8d`)

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

### Files
- `.github/workflows/build.yml`
- `.github/workflows/test.yml`
- 14 additional deleted workflow files under `.github/workflows/`

---

## 2026-01-09 - Applied Caspeco Patch (`dd80dc8c79b29b79fdfe3922db762e9e1f9a43f9`)

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

### Files
- `backend/plugins/github/models/deployment.go`
- `backend/plugins/github/models/migrationscripts/20251217_add_latest_status_update_date_to_deployment.go`
- `backend/plugins/github/models/migrationscripts/register.go`
- `backend/plugins/github_graphql/tasks/deployment_collector.go`
- `backend/plugins/github_graphql/tasks/deployment_extractor.go`
- `backend/plugins/github_graphql/tasks/deployment_convertor.go`
- `caspeco.patch`

---

## 2026-02-13 - Deployment exclusion (`f0ad0e8d3d2fe0d0a2a2dbe503179120cdf3f03d`)

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

### Files
- `backend/plugins/github_graphql/tasks/deployment_deduper.go`
- `backend/plugins/github_graphql/impl/impl.go`
- `backend/plugins/github_graphql/tasks/deployment_convertor.go`
- `backend/Dockerfile`
- `docker-compose.yml`
- `grafana/Dockerfile`
- `grafana/dashboards/DORADetails-ChangeFailureRate.json`
