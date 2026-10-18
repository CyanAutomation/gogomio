# Test contracts

These IDs identify the user-facing or operational behavior guarded by focused
tests. Each entry points to the existing documentation or implementation
contract that defines the behavior. Add an ID and source link when a new test
guards a behavior that does not already have a traceable requirement or issue.

| ID | Contract | Source |
| --- | --- | --- |
| `TC-CONN-01` | Concurrent client increments and decrements are all reflected in the connection count. | [Concurrency testing guidance](../../CONTRIBUTING.md#guidelines) |
| `TC-CONN-02` | Concurrent stream admission never exceeds the configured connection limit. | [Streaming behavior](../../README.md#features) |
| `TC-FRAME-01` | Frame waits return a published frame, or return no frame when canceled or timed out. | [WaitFrame documentation](../../internal/camera/frame_buffer.go) |
| `TC-FRAME-02` | Frame writes publish the latest bytes, advance the sequence, preserve mutable caller input, and record frames when the buffer creates its own stats collector. | [FrameBuffer documentation](../../internal/camera/frame_buffer.go) |
| `TC-FRAME-03` | Immutable frame writes publish the supplied storage without copying it; callers must not mutate it afterwards. | [WriteImmutable documentation](../../internal/camera/frame_buffer.go) |
| `TC-HTTP-01` | Snapshot responses contain a decodable JPEG; stream responses contain complete multipart JPEG frames. | [API reference](../reference/swagger.yaml) |
| `TC-DIAG-01` | Detailed health reports failures divided by all capture attempts (captured frames plus failures), including failures before the first frame. | [Detailed health API](../reference/swagger.yaml#/definitions/api.DetailedHealthResponse) |
| `TC-DIAG-02` | Detailed health classifies rates above 5% as degraded, and rates above 20% or more than five consecutive failures as poor. | [Detailed health API](../reference/swagger.yaml#/definitions/api.DetailedHealthResponse) |
| `TC-CAMERA-01` | Camera startup uses its first-frame deadline independently from the per-capture wait timeout. | [Real camera timeout contract](../../internal/camera/real_camera.go) |
| `TC-CAMERA-02` | Mock camera JPEG output reflects the configured dimensions and quality. | [Camera interface](../../internal/camera/camera_interface.go) |
| `TC-CAPTURE-01` | A camera panic is recovered by the background capture loop, which then marks itself stopped. | [Capture loop](../../internal/api/handlers.go) |
| `TC-CONFIG-01` | Frame timeout uses the configured frame interval, a 10 ms minimum, and a 5 second default. | [FrameTimeout documentation](../../internal/config/config.go) |
| `TC-CONFIG-02` | Invalid or nonpositive target FPS falls back to the configured capture FPS. | [Environment configuration](../../internal/config/config.go) |
| `TC-METRICS-01` | The published-frame counter matches the frames reported by stream statistics. | [Metrics handler](../../internal/api/handlers.go) |
| `TC-STATS-01` | Stream FPS is calculated from the latest 30 frame timestamps and is zero when those timestamps have no span. | [Stream statistics](../../internal/camera/stream_stats.go) |
| `TC-WEB-01` | The dashboard adapts to small screens and honors reduced-motion preferences. | [Responsive UI overview](../archive/PROJECT_SUMMARY.md) |
| `TC-WEB-02` | Stream aspect ratio comes from configured resolution dimensions, and invalid CSS dimensions are rejected. | [Aspect-ratio helper](../../internal/web/src/aspect-ratio.ts) |
| `TC-WEB-03` | Diagnostics dialog keyboard navigation stays inside the dialog and returns focus to its opener. | [Dialog behavior helper](../../internal/web/src/diagnostics-dialog.ts) |
| `TC-WEB-04` | Dashboard API requests use the documented paths and HTTP methods for configuration, diagnostics, and stopping a stream. | [API endpoints](../../README.md#api) |
| `TC-CI-01` | The coverage report upload is attempted even after the test job fails. | [Coverage workflow](../../.github/workflows/code-coverage-test.yml) |
| `TC-CI-03` | The coverage gate accepts totals at or above 75% and rejects below-threshold or malformed totals. | [Coverage guidance](../../CONTRIBUTING.md#guidelines) |
| `TC-DOC-01` | README and OpenAPI license labels match the canonical `LICENSE`, removal-date statements agree across docs, and documented API operations match registered Go handlers. | [Documentation contract check](../../scripts/test_documentation_contracts.py) |
