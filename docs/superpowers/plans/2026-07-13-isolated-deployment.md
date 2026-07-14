# Grok Auto Ban Isolated Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Safely validate and install `grok-autoban` on the production CPA instance without rewriting existing UTF-8 configuration through Windows text APIs.

**Architecture:** Add a byte-preserving configuration helper and test it locally, then use the same helper to create a disposable canary CPA configuration on the NAS. Load the candidate plugin in a canary container using the production image, verify registration and management behavior, and only then apply the proven artifacts to production with an automatic rollback path.

**Tech Stack:** Go 1.21, Python 3 standard library, Bash, Docker, CPA plugin ABI, CPA management HTTP API.

---

## File Structure

- Create `scripts/configure_plugin.py`: byte-preserving insertion and removal of the plugin YAML block.
- Create `scripts/test_configure_plugin.py`: regression tests for UTF-8 preservation, idempotency, and removal.
- Create `scripts/canary-deploy.sh`: create and operate the isolated CPA canary container.
- Modify `README.md`: document safe installation and rollback commands.

### Task 1: Byte-Preserving Configuration Helper

**Files:**
- Create: `scripts/configure_plugin.py`
- Create: `scripts/test_configure_plugin.py`

- [ ] **Step 1: Write failing UTF-8 preservation tests**

Create tests that build a YAML fixture containing Chinese provider names and a
`plugins.configs.grok-inspection` block. Test these exact behaviors:

```python
def test_enable_preserves_existing_bytes():
    original = FIXTURE.encode("utf-8")
    updated = configure(original, enabled=True)
    assert "英伟达".encode("utf-8") in updated
    assert updated.count(b"    grok-autoban:\n") == 1


def test_enable_is_idempotent():
    once = configure(FIXTURE.encode("utf-8"), enabled=True)
    twice = configure(once, enabled=True)
    assert twice == once


def test_disable_removes_only_plugin_block():
    enabled = configure(FIXTURE.encode("utf-8"), enabled=True)
    disabled = configure(enabled, enabled=False)
    assert disabled == FIXTURE.encode("utf-8")
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```powershell
python -m unittest scripts.test_configure_plugin -v
```

Expected: import or missing-function failure because
`scripts/configure_plugin.py` does not exist.

- [ ] **Step 3: Implement byte-level configuration editing**

Implement `configure(data: bytes, enabled: bool) -> bytes` using ASCII byte
anchors. Do not decode or re-encode the full CPA file. Insert this exact block
immediately before the next four-space plugin key after `grok-inspection`:

```yaml
    grok-autoban:
      enabled: true
      priority: 100
      fallback_hours: 24
      persist_state: true
      state_file: plugins/data/grok-autoban/bans.json
      log_matches: true
```

The CLI must support:

```text
python3 configure_plugin.py enable INPUT OUTPUT
python3 configure_plugin.py disable INPUT OUTPUT
```

Write through a temporary file and `os.replace` when input and output paths are
the same. Preserve the original newline style.

- [ ] **Step 4: Run tests and verify success**

Run:

```powershell
python -m unittest scripts.test_configure_plugin -v
```

Expected: 3 tests pass.

- [ ] **Step 5: Commit the helper**

```powershell
git add scripts/configure_plugin.py scripts/test_configure_plugin.py
git commit -m "fix: preserve CPA config bytes during plugin setup"
```

### Task 2: Verify Plugin Source and Linux Artifact

**Files:**
- Verify: `*.go`
- Verify: `plugins-disabled/grok-autoban.so` on the NAS

- [ ] **Step 1: Run the full plugin test suite**

Run:

```powershell
go test ./...
go vet ./...
```

Expected: all packages pass and `go vet` exits 0.

- [ ] **Step 2: Rebuild the Linux plugin in the existing Go Docker image**

On the NAS, mount `/volume1/docker/cpa` at `/src` and run:

```bash
docker run --rm \
  -v /volume1/docker/cpa:/src \
  -w /src/plugins-src/grok-autoban \
  docker.m.daocloud.io/library/golang:1.26-bookworm \
  bash /src/plugins-src/grok-autoban/docker-build.sh
```

Write the result to the quarantine location first, not the production plugin
directory. Confirm `file` reports an x86-64 ELF shared object.

- [ ] **Step 3: Record the artifact checksum**

Run:

```bash
sha256sum /volume1/docker/cpa/plugins-disabled/grok-autoban.so
```

Expected: one SHA-256 checksum for the candidate artifact.

### Task 3: Canary Deployment Script

**Files:**
- Create: `scripts/canary-deploy.sh`

- [ ] **Step 1: Implement canary preparation**

The script must create
`/volume1/docker/cpa/canary/grok-autoban`, copy production configuration and
auth data, create a dedicated plugin tree, and copy existing production plugins
plus the candidate plugin. It must call `configure_plugin.py enable` on the
canary copy only.

- [ ] **Step 2: Implement canary container lifecycle**

Use the exact image returned by:

```bash
docker inspect cli-proxy-api --format '{{.Config.Image}}'
```

Start `cli-proxy-api-grok429-canary` with these mounts:

```text
canary config.yaml -> /CLIProxyAPI/config.yaml
canary auth-dir    -> /root/.cli-proxy-api
canary plugins     -> /CLIProxyAPI/plugins
```

Bind an unused host port to container port `8317`, pass the existing management
password through the environment, and use restart policy `no`.

- [ ] **Step 3: Implement canary cleanup**

Support `stop` and `clean` commands. `clean` must refuse to remove any path
outside `/volume1/docker/cpa/canary/grok-autoban`.

- [ ] **Step 4: Shell syntax check and commit**

Run:

```bash
bash -n scripts/canary-deploy.sh
```

Expected: exit 0.

Commit:

```powershell
git add scripts/canary-deploy.sh
git commit -m "feat: add isolated CPA plugin canary deployment"
```

### Task 4: Canary Compatibility Verification

**Files:**
- No source changes unless a specific compatibility failure is proven.

- [ ] **Step 1: Start the canary**

Run `scripts/canary-deploy.sh start` on the NAS and record the selected port.

- [ ] **Step 2: Verify process stability and registration**

Check container state twice at least 10 seconds apart. Both checks must report
`running` and restart count `0`. Logs must contain:

```text
plugin loaded plugin_id=grok-autoban
plugin registered plugin_id=grok-autoban
API server started successfully
```

- [ ] **Step 3: Verify CPA HTTP and plugin management surfaces**

Confirm:

```text
GET /v1/models                                             -> 200
GET /v0/resource/plugins/grok-autoban/status           -> 200
GET /v0/management/plugins/grok-autoban/bans           -> 200 JSON
```

Use the existing API key for `/v1/models` and the management password for the
management route.

- [ ] **Step 4: Verify synthetic plugin behavior**

Run the focused Go tests for:

```text
subscription:free-usage-exhausted with HTTP 429
permission-denied with HTTP 403
scheduler removal of banned xAI auth IDs
manual unban
automatic expiration
persistent reload
```

Expected: all focused tests pass. Do not intentionally send failing production
accounts through the canary.

- [ ] **Step 5: Restart canary and verify persistence**

Create a synthetic stored ban, restart the canary, and verify the bans endpoint
still returns the entry. Clear it through `unban-all` afterward.

### Task 5: Production Rollout and Automatic Rollback

**Files:**
- Modify: `/volume1/docker/cpa/config.yaml`
- Install: `/volume1/docker/cpa/plugins/linux/amd64/grok-autoban.so`

- [ ] **Step 1: Create byte-for-byte production backups**

Copy `config.yaml` and the current plugin directory to timestamped backup paths
on the NAS. Record the candidate plugin SHA-256 checksum.

- [ ] **Step 2: Install the verified artifact and configuration**

Copy the canary-tested `.so` into `plugins/linux/amd64`. Run
`configure_plugin.py enable` directly on production `config.yaml` using atomic
replacement.

- [ ] **Step 3: Restart production and run the acceptance checks**

Restart only `cli-proxy-api`. Check twice at least 10 seconds apart:

```text
container state = running
restart count = 0
/v1/models = HTTP 200
logs include grok-autoban loaded and registered
bans management endpoint = HTTP 200 JSON
```

- [ ] **Step 4: Roll back on any failed acceptance check**

If any check fails, restore the byte-for-byte config backup, move the candidate
plugin to `plugins-disabled`, restart CPA, and verify production returns to
`running`, restart count `0`, and `/v1/models` HTTP 200.

- [ ] **Step 5: Stop and clean the canary**

After production passes, remove only the canary container and canary directory.
Keep the production rollback backup.

### Task 6: Documentation and Final Verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Document safe installation and rollback**

Add commands for the byte-preserving configuration helper, canary deployment,
production acceptance checks, and rollback. Explicitly warn against reading and
rewriting CPA YAML with Windows PowerShell's default encoding.

- [ ] **Step 2: Run all repository checks**

Run:

```powershell
go test ./...
go vet ./...
python -m unittest scripts.test_configure_plugin -v
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 3: Commit documentation**

```powershell
git add README.md
git commit -m "docs: add safe CPA deployment workflow"
```

- [ ] **Step 4: Record final production evidence**

Report the production container state, restart count, plugin version, artifact
checksum, `/v1/models` status, management endpoint status, and rollback backup
path.
