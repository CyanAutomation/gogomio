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
| `TC-HTTP-01` | Snapshot responses contain a decodable JPEG; stream responses contain complete multipart JPEG frames. | [API reference](../reference/swagger.yaml) |
| `TC-CAMERA-01` | Camera startup uses its first-frame deadline independently from the per-capture wait timeout. | [Real camera timeout contract](../../internal/camera/real_camera.go) |
| `TC-CONFIG-01` | Frame timeout uses the configured frame interval, a 10 ms minimum, and a 5 second default. | [FrameTimeout documentation](../../internal/config/config.go) |
| `TC-WEB-01` | The dashboard adapts to small screens and honors reduced-motion preferences. | [Responsive UI overview](../archive/PROJECT_SUMMARY.md) |
| `TC-WEB-02` | Stream aspect ratio comes from configured resolution dimensions, and invalid CSS dimensions are rejected. | [Aspect-ratio helper](../../internal/web/aspect-ratio.js) |
| `TC-CI-01` | The coverage report upload is attempted even after the test job fails. | [Coverage workflow](../../.github/workflows/code-coverage-test.yml) |
