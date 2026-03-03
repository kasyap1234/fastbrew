# fastbrewd

`fastbrewd` is an optional long-lived local daemon used to accelerate read-heavy FastBrew commands and keep hot in-memory state between CLI invocations.

## Rollout Model

- Default: `daemon.enabled=false` for safe rollout.
- FastBrew CLI is always the UX entrypoint.
- If daemon is enabled but unavailable or version-mismatched, commands automatically fall back to local mode.

## Socket + Security

- Socket path: `~/.fastbrew/run/daemon.sock` (configurable).
- Socket permissions: `0600`.
- Client validates socket owner UID and mode before connecting.
- Non-socket paths are rejected; stale socket files are cleaned up safely.

## Managed State

The daemon holds hot state for:

- Formula/cask/search index access paths.
- Prefix index usage for fuzzy search.
- Installed package snapshot cache.
- Outdated snapshot cache.
- Formula/cask metadata LRU + TTL cache.
- Tap info and services list caches.
- Dependency and leaves result caches.

## Cache Coherence

Mutation paths publish invalidation events:

- `installed_changed`
- `tap_changed`
- `index_refreshed`
- `service_changed`

These events clear only affected cache groups so read commands stay fast without stale results.

## Command Surface

Daemon management commands:

- `fastbrew daemon start`
- `fastbrew daemon stop`
- `fastbrew daemon status`
- `fastbrew daemon stats`
- `fastbrew daemon warmup`

The foreground daemon process is exposed as hidden command `fastbrew daemon serve`.

Mutation commands can execute as daemon jobs when daemon is enabled:

- `fastbrew install`
- `fastbrew upgrade`
- `fastbrew uninstall`
- `fastbrew reinstall`

CLI submits a job and streams daemon job events back to the terminal. If submission fails, CLI falls back to local execution.

`job_stream` supports blocking mode (`blocking=true`) for long-poll behavior. In blocking mode, the daemon waits until:

- New events are available, or
- The job reaches a terminal state (`succeeded` / `failed`), or
- A 30 second heartbeat timeout is reached.

This significantly reduces polling chatter versus fixed-interval polling.

Job events remain backward compatible with `level` and `message`, and now include optional structured fields:

- `kind` (`job` or `package`)
- `operation`
- `package`
- `phase` (`resolve`, `metadata`, `download`, `extract`, `link`, `install`, `uninstall`, `complete`)
- `status` (`queued`, `running`, `progress`, `succeeded`, `failed`, `skipped`)
- `current`, `total`, `unit` (for progress telemetry, typically bytes)

## Config

Config keys under `daemon`:

- `daemon.enabled` (`bool`)
- `daemon.auto_start` (`bool`)
- `daemon.idle_timeout` (`duration`, e.g. `15m`)
- `daemon.socket_path` (`string`)
- `daemon.prewarm` (`bool`)

## Reliability Behavior

- Idle timeout auto-shutdown is supported.
- Auto-start on demand when enabled and `auto_start=true`.
- Version handshake validates `api_version` and `binary_version`.
- Version mismatch triggers graceful fallback to local execution.
