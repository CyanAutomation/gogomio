# GoGoMio Skills

These repository skills give GitHub Copilot focused guidance for specialized GoGoMio work. Each skill is selected when relevant; follow the current implementation and the user's request if either differs from an example or default.

| Skill | Use when |
| --- | --- |
| [Go Concurrency Patterns](./go-concurrency-patterns/SKILL.md) | Changing goroutine lifecycles, shared frame state, channel ownership, or shutdown behavior. |
| [HTTP Streaming Patterns](./http-streaming-patterns/SKILL.md) | Changing MJPEG endpoints, multipart framing, stream limits, or client cancellation. |
| [Camera Integration Patterns](./camera-integration-patterns/SKILL.md) | Changing camera backends, subprocess handling, startup fallback, or health reporting. |
| [Embedded Deployment](./embedded-iot-deployment/SKILL.md) | Deploying to Raspberry Pi or changing resource limits, Docker images, or architecture targets. |
| [Go CLI Design](./go-cli-design/SKILL.md) | Changing the CLI dispatcher, commands, API client, help, or output. |
| [Front-end Design](./front-end-design/SKILL.md) | Redesigning the visual hierarchy or interaction design of a web page or app surface. |
| [API and Settings Patterns](./api-and-settings-patterns/SKILL.md) | Changing HTTP routes, middleware, OpenAPI contracts, or persistent settings behavior. |
| [HTTP Security and Authentication](./http-security-and-authentication/SKILL.md) | Changing authentication, authorization boundaries, CORS, same-origin checks, proxy trust, or HTTP rate limiting. |
| [Web UI Engineering](./web-ui-engineering/SKILL.md) | Implementing or changing GoGoMio browser behavior, API integration, UI tests, or embedded web assets. |
| [Configuration and Validation](./configuration-and-validation/SKILL.md) | Changing environment configuration, defaults, validation, or startup configuration contracts. |
| [CI and Container Workflows](./ci-container-workflows/SKILL.md) | Changing GitHub Actions, CI helper scripts, Docker builds, image publication, or architecture verification. |

## Keep skills current

- Describe current repository behavior. Label proposed future behavior as a proposal, not as an existing default.
- Keep each `SKILL.md` focused on when it applies and the project-specific decisions it changes. Link to implementation, tests, or a focused reference for detail.
- Keep each frontmatter `name` aligned with its skill directory and keep this catalog in sync.
- Keep documented `go test -run` selectors matched to actual test functions.
- Run the skill checks after editing:

  ```bash
  python3 scripts/check_skills.py
  python3 -B -m unittest discover -s scripts -p 'test_*.py'
  ```

See [CLAUDE.md](../../CLAUDE.md) for the project overview, [CONTRIBUTING.md](../../CONTRIBUTING.md) for contribution guidance, and the linked implementation and deployment docs for current details.
