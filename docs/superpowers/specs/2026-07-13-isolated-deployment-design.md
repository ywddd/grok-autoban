# Grok Auto Ban Isolated Deployment Design

## Goal

Install `grok-autoban` into the production CPA instance without risking the
existing service. Prove plugin compatibility and behavior in an isolated CPA
container before changing the production plugin directory or configuration.

## Current State

- Production CPA is healthy on port `8317` with restart count `0`.
- The candidate Linux plugin is quarantined at
  `plugins-disabled/grok-autoban.so`.
- The previous restart loop was caused by rewriting the UTF-8 CPA YAML through
  Windows PowerShell's default text encoding. The plugin binary was not loaded
  during that failure.
- Production configuration has been restored from the byte-for-byte backup.

## Approach

Create a disposable CPA canary container using the same CPA image as
production, a byte-for-byte copy of the current configuration, a separate
plugin directory, a separate writable auth directory, and an unused host port.
Only the canary receives the candidate plugin and plugin configuration.

The production container remains running throughout canary testing.

## Configuration Safety

All YAML modifications must be performed on the NAS with a UTF-8-aware script.
The script must:

1. Read and write UTF-8 without transcoding existing bytes through Windows
   PowerShell defaults.
2. Insert only the `grok-autoban` configuration block under
   `plugins.configs`.
3. Validate the resulting YAML by starting the canary and checking its logs.
4. Never overwrite the production configuration during canary testing.

The production rollout must create a timestamped byte-for-byte backup before
editing `config.yaml`.

## Canary Layout

- Config: `canary/grok-autoban/config.yaml`
- Plugins: `canary/grok-autoban/plugins`
- Auth directory: `canary/grok-autoban/auth-dir`
- Container: `cli-proxy-api-grok429-canary`
- Host port: choose an unused port at runtime
- Image: exactly the image used by `cli-proxy-api`

The canary auth directory is copied from production only for startup fidelity.
Tests must not send real upstream requests through the canary unless explicitly
required. Synthetic plugin method tests are preferred for 429 and 403 behavior.

## Verification

The canary must pass all of the following checks:

1. Container remains running with restart count `0`.
2. Logs show `grok-autoban` loaded and registered at version `0.1.1`.
3. Existing plugins still load successfully.
4. `/v1/models` returns HTTP 200 with a configured API key.
5. The plugin resource page returns HTTP 200.
6. The authenticated bans management endpoint returns valid JSON.
7. Synthetic free-usage 429 creates a temporary ban with the expected reset
   time and removes the account from scheduler candidates.
8. Synthetic permission-denied 403 creates a manual-unban entry and removes the
   account from scheduler candidates.
9. Unban and unban-all management operations clear matching entries.
10. Plugin state persists across a canary restart when persistence is enabled.

## Production Rollout

After all canary checks pass:

1. Copy the candidate plugin into `plugins/linux/amd64`.
2. Back up production `config.yaml` byte-for-byte.
3. Add the validated plugin block using the same UTF-8-safe mechanism.
4. Restart only `cli-proxy-api`.
5. Confirm restart count remains `0`, `/v1/models` returns HTTP 200, and logs
   show successful plugin registration.
6. Confirm the plugin management endpoints respond correctly.

## Rollback

If any production verification fails:

1. Restore the byte-for-byte configuration backup.
2. Move `grok-autoban.so` back to `plugins-disabled`.
3. Restart `cli-proxy-api`.
4. Confirm restart count `0` and `/v1/models` HTTP 200.

## Scope

This deployment does not change the plugin's classification rules or user
interface. Source changes are allowed only when isolated verification proves a
specific compatibility defect, and each such change requires a focused test.
