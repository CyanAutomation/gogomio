# MIO Sprite Migration Reference

This document records the sprite filename migration used by the embedded web UI.

## Canonical Runtime Sprite Set

Sprites served from `/static/mio/` are sourced from `internal/web/mio/` and currently use pose-based names:

- `mio_pose_idle.png`
- `mio_pose_sleeping.png`
- `mio_pose_concerned.png`
- `mio_pose_happy.png`
- `mio_pose_worried.png`
- `mio_pose_curious.png`
- `mio_pose_angry.png`
- `mio_pose_looking.png`

## Replaced Legacy Filenames

The following legacy filenames were retired from `internal/web/mio/` and are expected to return 404:

- `mio_avatar.png`
- `mio_curious.png`
- `mio_sleeping.png`
- `mio_happy.png`

## Current Web UI Mapping

The web UI maps runtime state to pose assets in `internal/web/index.html`:

- `idle` -> `mio_pose_sleeping.png`
- `loading` -> `mio_pose_concerned.png`
- `success` -> `mio_pose_happy.png`
- `warning` -> `mio_pose_concerned.png`
- `running` -> `mio_pose_worried.png`

## Validation

Coverage is enforced by tests in `internal/web/web_test.go`:

- `TestMioStaticAssetsAreServed`
- `TestLegacyMioStaticAssetsAreNotServed`

Run:

```bash
go test ./internal/web
```