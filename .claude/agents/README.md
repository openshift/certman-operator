# Claude Agents

Specialized agents for this operator development workflows.

## Available Agents

### [lint-agent](./lint-agent.md)
**Purpose**: Automated linting and code quality enforcement

**When to use**:
- Pre-commit validation
- After code generation
- Before creating PR
- Investigating CI lint failures

**Key capabilities**:
- Run formatting checks
- Execute golangci-lint
- Auto-fix safe issues
- Report unfixable problems

---

### [test-agent](./test-agent.md)
**Purpose**: Automated testing and test quality assurance

**When to use**:
- After code changes
- Test failures in CI
- Before creating PR
- After regenerating mocks

**Key capabilities**:
- Run targeted tests for changed code
- Detect flaky test failures
- Suggest minimal fixes
- Ensure test coverage

---

### [security-agent](./security-agent.md)
**Purpose**: Security scanning and policy enforcement

**When to use**:
- Before committing code
- RBAC manifests modified
- Secret handling changed
- CI/CD pipelines modified

**Key capabilities**:
- Scan for hardcoded secrets
- Validate RBAC configurations
- Check insecure patterns
- Enforce security policies

---

### [docs-agent](./docs-agent.md)
**Purpose**: Documentation maintenance and synchronization

**When to use**:
- Code changes affect workflows
- New features added
- Build process modified
- Command examples need updating

**Key capabilities**:
- Update docs after code changes
- Ensure command examples work
- Validate markdown formatting
- Keep docs synchronized

---

### [ci-agent](./ci-agent.md)
**Purpose**: CI/CD validation and workflow integrity

**When to use**:
- Tekton pipelines modified
- Pre-commit hooks changed
- CI failures need investigation
- New validation steps added

**Key capabilities**:
- Validate pipeline integrity
- Ensure local/CI parity
- Detect missing checks
- Optimize execution order

---

## Usage Patterns

### Single Agent Invocation
Use a specific agent when the task is clear:
```text
"Run lint-agent to check formatting"
"Use security-agent to scan for secrets"
"Invoke test-agent on controllers/certificaterequest"
```

### Multi-Agent Workflow
Agents can work together for comprehensive validation:
```text
1. lint-agent: Fix formatting and linting
2. test-agent: Run affected tests
3. security-agent: Scan for secrets and RBAC issues
4. docs-agent: Update documentation
5. ci-agent: Validate CI parity
```

### Pre-Commit Workflow
Recommended agent sequence before committing:
```text
1. security-agent (secrets, RBAC)
2. lint-agent (formatting, linting)
3. test-agent (targeted tests)
4. docs-agent (if docs need updates)
```

### Pre-PR Workflow
Full validation before creating pull request:
```text
1. lint-agent --all-files
2. test-agent --full-suite
3. security-agent --comprehensive
4. docs-agent --validate
5. ci-agent --parity-check
```

## Agent Design Principles

All agents follow these principles:

**Focused Responsibility**
- Each agent has clear, narrow responsibilities
- No overlap with other agents
- Single purpose, well-defined scope

**Reuse Existing Tools**
- Leverage pre-commit hooks
- Use Makefile targets
- Don't reinvent validations

**Fast Feedback**
- Quick execution (<30s for targeted checks)
- Fail fast on common issues
- Provide actionable output

**CI Parity**
- Mirror CI checks locally
- Use same tool versions
- Deterministic results

**Safe Automation**
- Auto-fix only safe changes
- Escalate risky modifications
- Never bypass security checks

**Clear Escalation**
- Define when human intervention needed
- Explain what can't be auto-fixed
- Provide context for decisions

## Integration with Pre-commit

Agents complement (don't replace) pre-commit hooks:

| Pre-commit Hook | Corresponding Agent |
|-----------------|---------------------|
| `gitleaks` | security-agent |
| `golangci-lint` | lint-agent |
| `go-build` | lint-agent |
| `go-mod-tidy` | lint-agent |
| `rbac-wildcard-check` | security-agent |

**Relationship:**
- Pre-commit hooks: Automated git hooks (mandatory)
- Agents: Interactive assistance (on-demand)
- Both use same underlying tools

## Output Format

All agents should report findings consistently:
```text
[AGENT] [SEVERITY] Location: Issue
Example: [lint-agent] [ERROR] pkg/handler/deployment.go:42: unreachable code
Example: [security-agent] [CRITICAL] deploy/role.yaml:15: Wildcard permission
```

Severity levels:
- **CRITICAL**: Blocks commit/PR
- **ERROR**: Must fix before merge
- **WARNING**: Should fix
- **INFO**: Informational

## Extension Guide

To add a new agent:

1. Create `new-agent.md` in this directory
2. Add YAML frontmatter at the top:
   ```yaml
   ---
   name: new-agent
   description: Brief description of when to use this agent. Be specific about use cases.
   tools: Bash, Read, Edit, Grep
   model: sonnet
   ---
   ```
3. Follow the template structure in markdown body:
   - **Responsibilities**: What it does
   - **Usage**: When to invoke
   - **Commands**: How it works
   - **Escalation**: When to defer to human
4. Update this README with agent description
5. Test agent workflows locally
6. Document integration points

**Required frontmatter fields**:
- `name`: Agent identifier (kebab-case, matches filename)
- `description`: When to use this agent (triggers invocation)
- `tools`: Comma-separated list of allowed tools
- `model`: Claude model to use (`sonnet`, `opus`, or `haiku`)

**Agent file structure**:
```text
.claude/agents/
├── README.md
├── ci-agent.md           # Frontmatter + markdown body
├── docs-agent.md
├── lint-agent.md
├── security-agent.md
└── test-agent.md
```

## Agent Communication

Agents can reference each other:
- `lint-agent` may suggest running `test-agent`
- `security-agent` may trigger `ci-agent` for pipeline validation
- `docs-agent` updates after `lint-agent` or `test-agent` changes

Keep communication minimal and explicit.
