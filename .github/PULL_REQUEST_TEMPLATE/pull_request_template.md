## Description

<!-- Explain what this PR does and why. Reference related issues with "Closes #123". -->

## Type of Change

- [ ] Bug fix
- [ ] New feature
- [ ] Refactor (no behaviour change)
- [ ] Performance improvement
- [ ] Documentation
- [ ] Test additions/updates
- [ ] Build / CI / chore

## Checklist

- [ ] All tests pass locally: `go test ./... -v -race -cover`
- [ ] Coverage is ≥75%: `go tool cover -func=coverage.out | grep total`
- [ ] No new race conditions introduced (ran with `-race`)
- [ ] CHANGELOG.md updated (if this is a user-visible change)
- [ ] Public APIs have Godoc comments
- [ ] PR title follows conventional commits: `<type>: <description>`
