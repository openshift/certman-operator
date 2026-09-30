# Contributing to Certman

Thank you for your interest in contributing to the Certman project.

## Quick Start

1. **Setup**: Install Go 1.22.7+, operator-sdk v1.21.0
2. **Install tools**: `make tools`
3. **Run pre-commit**: `pip install pre-commit && pre-commit install`
4. **Build**: `make go-build`
5. **Test**: `make go-test`
6. **Lint**: `make go-check`

See [DEVELOPMENT.md](./DEVELOPMENT.md) for detailed setup instructions.

## Before Submitting a PR

All contributions must pass:

1. **Formatting & linting**: `pre-commit run --all-files`
2. **Unit tests**: `make go-test`
3. **Build verification**: `make go-build`
4. **Security scan**: Automatic via pre-commit (gitleaks)

## Development Workflow

### Human Contributors

```bash
# Create a feature branch
git checkout -b feature/my-change

# Make changes, following existing code patterns
# Add/update tests for your changes

# Run validation locally
pre-commit run --all-files
make go-test

# Commit with descriptive message
git commit -m "feat: add support for X"

# Push and create PR
git push origin feature/my-change
```

### AI-Assisted Development

When using AI coding agents (Claude Code, GitHub Copilot, Cursor, etc.):

**Agents MUST:**
- Run `pre-commit run` on changed files before committing
- Execute relevant tests after code changes: `make go-test`
- Preserve existing code style and patterns
- Avoid editing generated files (`**/zz_generated.*.go`, `go.sum` without `go.mod`)
- Never bypass hooks with `--no-verify`
- Never commit secrets, tokens, or credentials
- Reuse existing utilities and abstractions
- Make incremental, focused changes

**Validation expectations:**
1. Format check: `go fmt ./...`
2. Lint: `make go-check` (or `pre-commit run golangci-lint`)
3. Type safety: Verified by `go build ./...` in pre-commit
4. Tests: `make go-test` for affected packages
5. Secret scan: Automatic via pre-commit gitleaks hook

**Required checks before PR:**
- [ ] All pre-commit hooks pass
- [ ] Unit tests pass for modified packages
- [ ] No new linter warnings introduced
- [ ] No secrets or credentials in diff
- [ ] Mocks regenerated if interfaces changed: `boilerplate/_lib/container-make generate`

## Code Style

Follow existing patterns:
- Standard Go formatting (`gofmt`)
- golangci-lint rules in `boilerplate/openshift/golang-osd-operator/golangci.yml`
- Ginkgo/Gomega for tests
- GoMock for interface mocking

## Testing Requirements

- **Unit tests required** for all new functionality
- Use Ginkgo BDD style: `Describe`, `Context`, `It`
- Mock external dependencies with GoMock
- Aim for meaningful test coverage, not just metrics

See [TESTING.md](./TESTING.md) for testing guidelines.

## Regenerating Code

After modifying API types or interfaces:

```bash
# Regenerate deepcopy, OpenAPI, mocks (in container for consistency)
boilerplate/_lib/container-make generate
```

## Security

**Never commit:**
- API keys, tokens, passwords
- AWS credentials, kubeconfig files
- Private keys, certificates
- `.env` files with secrets
- Debug statements printing sensitive data

The pre-commit gitleaks hook will block commits containing secrets.

**High-risk changes** (requiring extra review):
- Authentication/authorization logic
- RBAC manifests with wildcard permissions
- Network policies
- CI/CD pipeline modifications
- Dockerfile changes

## Commit Message Format

Use conventional commits style:

```text
<type>: <short summary>

<optional body>

<optional footer>
```

Types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`, `ci`

Examples:
- `feat: add support for fleet notification filtering`
- `fix: correct RBAC permissions for service monitor`
- `test: add unit tests for network policy handler`

## Pull Request Process

1. **Title**: Clear, descriptive summary
2. **Description**: Explain what changed and why
3. **Testing**: Describe how you tested the changes
4. **CI**: All Tekton pipeline checks must pass
5. **Review**: Address review feedback promptly

## Questions?

- Check existing documentation in [docs/](./docs/)
- Review similar PRs for patterns
- Ask in PR comments for clarification

## License

All contributions are licensed under Apache 2.0. See [LICENSE](./LICENSE).
