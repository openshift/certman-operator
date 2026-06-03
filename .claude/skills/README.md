# Claude Skills

Reusable workflow skills for this operator development.

## Available Skills

### [prow-ci](./prow-ci/SKILL.md)
**Purpose**: Access and analyze OpenShift Prow CI results

**When to use**:
- Investigating CI failures
- Checking test results
- Analyzing build logs
- Debugging failed PR checks

**Key capabilities**:
- Access Prow dashboard and job results
- Retrieve build logs and artifacts
- Debug test failures
- Compare local vs CI results

**Resources**:
- [Prow Dashboard](https://prow.ci.openshift.org/)
- [CI Search](https://github.com/openshift/ci-search)

## Usage

Skills are reusable workflows that combine multiple tools and knowledge to accomplish specific tasks.

### Invoking Skills

Skills can be referenced in Claude conversations:
```text
"Use the prow-ci skill to investigate the failed test in PR #123"
"Check Prow CI results for the latest build"
```

### Skill Components

Each skill typically includes:
- **Purpose**: What the skill does
- **Usage**: When to invoke it
- **Commands**: Specific commands to run
- **Troubleshooting**: Common issues and solutions
- **Integration**: How it works with other tools

## Creating New Skills

To add a new skill:

1. Create subdirectory: `skillname/` in this directory
2. Create `SKILL.md` inside the subdirectory
3. Use frontmatter with metadata:
   ```yaml
   ---
   name: skillname
   description: Brief description of what this skill does
   trigger: skill triggers, slash command synonyms
   ---
   ```
4. Document commands and workflows in the markdown body
5. Update this README
6. Test the skill workflow

**Directory structure**:
```text
.claude/skills/
├── README.md
└── skillname/
    ├── SKILL.md          # Required: skill definition
    └── reference/        # Optional: supporting docs
```

## Integration with Other Components

**Skills vs Agents**:
- **Agents**: Autonomous actors with specific responsibilities
- **Skills**: Reusable workflows that agents or humans execute

**Skills vs Hooks**:
- **Hooks**: Automated enforcement (runs automatically)
- **Skills**: On-demand workflows (runs when invoked)

**Skills vs Commands**:
- **Commands**: Simple, single-purpose actions
- **Skills**: Complex, multi-step workflows

## Future Skills

Planned skills for this repository:

### dependency-update
- Check for outdated dependencies
- Update go.mod
- Run tests
- Validate compatibility
- Create update PR

### release-prep
- Update version numbers
- Generate changelog
- Run full validation
- Create release PR
- Tag release

### api-compat-check
- Compare API changes
- Detect breaking changes
- Suggest migration path
- Update documentation

### security-audit
- Run gitleaks on full history
- Check dependencies for CVEs
- Review RBAC configurations
- Scan container images
- Generate security report

## References

- [CLAUDE.md](../../CLAUDE.md) - Agent behavioral rules
- [.claude/agents/](../agents/) - Specialized agents
- [.claude/hooks/](../hooks/) - Security and validation hooks
