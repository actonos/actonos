---
name: actonos-release
description: "Skill for creating ActonOS releases. Covers version bumping, changelog management, git tagging, Docker image publishing, and ISO creation."
---

# ActonOS Release Skill

Use this skill when creating releases for ActonOS — bumping versions, generating changelogs, tagging, and publishing artifacts.

## Release Checklist

Before creating a release, ensure:

- [ ] All tests pass: `make test`
- [ ] Linters pass: `make lint`
- [ ] Build succeeds: `make build`
- [ ] `CHANGELOG.md` has entries under `[Unreleased]`
- [ ] Documentation is up to date
- [ ] No critical open issues for this version

## Version Bump

### Quick Commands

```bash
# Patch release (bug fixes): 0.1.0 → 0.1.1
make bump-patch

# Minor release (new features): 0.1.0 → 0.2.0
make bump-minor

# Major release (breaking changes): 0.1.0 → 1.0.0
make bump-major
```

### What Happens

The `scripts/version-bump.sh` script automatically:

1. **Reads** current version from `VERSION` file
2. **Calculates** new version
3. **Updates** `VERSION` file
4. **Updates** `CHANGELOG.md`:
   - Moves `[Unreleased]` entries to `[X.Y.Z] - YYYY-MM-DD`
   - Creates fresh `[Unreleased]` section
   - Updates comparison links
5. **Creates** git commit: `chore(release): vX.Y.Z`
6. **Creates** annotated git tag: `vX.Y.Z`

### Dry Run

Preview without modifying files:

```bash
bash scripts/version-bump.sh patch --dry-run
```

## Changelog Management

### Before Release

Add entries to `CHANGELOG.md` under `[Unreleased]`:

```markdown
## [Unreleased]

### Added
- Multi-agent swarm delegation with configurable timeout (#42)
- Discord channel adapter (#38)

### Fixed
- FTS5 index corruption on concurrent writes (#35)
- Token refresh daemon race condition (#33)

### Changed
- Improved agent manifest validation error messages
```

### Auto-Generate from Commits

```bash
# Generate entries from conventional commits since last tag
bash scripts/changelog-gen.sh

# Since a specific tag
bash scripts/changelog-gen.sh v0.1.0

# All commits
bash scripts/changelog-gen.sh --all
```

The script groups commits by type:
- `feat` → **Added**
- `fix` → **Fixed**
- `refactor`, `docs`, `style`, `test`, `build`, `ci`, `chore` → **Changed**
- Breaking changes (`!`) → **Removed** (with ⚠️ marker)

## Full Release Workflow

### Step 1: Prepare

```bash
# Ensure you're on main with latest changes
git checkout main
git pull origin main

# Verify everything passes
make lint
make test
make build
```

### Step 2: Review Changelog

```bash
# Auto-generate changelog entries
bash scripts/changelog-gen.sh

# Review and edit CHANGELOG.md manually
# Add any missing entries, polish descriptions
```

### Step 3: Bump Version

```bash
# Choose the appropriate bump type
make bump-minor  # or bump-patch / bump-major
```

### Step 4: Push

```bash
# Push commit and tag
git push origin main --tags
```

### Step 5: Build & Publish Artifacts

```bash
# Docker image
make docker
docker push actonos/agent:$(cat VERSION)
docker push actonos/agent:latest

# ISO (if releasing bare-metal)
make iso
```

### Step 6: Create GitHub Release

1. Go to GitHub → Releases → "Create new release"
2. Select the tag `vX.Y.Z`
3. Title: `ActonOS vX.Y.Z`
4. Body: Copy the changelog section for this version
5. Attach artifacts:
   - `build/actond` (Linux AMD64 binary)
   - `build/ActonOS-vX.Y.Z.iso` (if applicable)

## Hotfix Release

For urgent fixes on a released version:

```bash
# 1. Create hotfix branch from the tag
git checkout -b hotfix/v0.2.1 v0.2.0

# 2. Apply the fix
# ... make changes ...
git commit -m "fix(memory): critical FTS5 corruption on write"

# 3. Bump patch version
bash scripts/version-bump.sh patch

# 4. Merge back to main
git checkout main
git merge hotfix/v0.2.1

# 5. Push
git push origin main --tags

# 6. Clean up
git branch -d hotfix/v0.2.1
```

## Version Files Reference

| File | Purpose | Updated By |
|:---|:---|:---|
| `VERSION` | Source of truth | `scripts/version-bump.sh` |
| `CHANGELOG.md` | Release history | Manual + `scripts/version-bump.sh` |
| Go binary `-ldflags` | Embedded version | `Makefile` (reads `VERSION`) |
| Docker tag | Image version | `Makefile` (reads `VERSION`) |
| ISO filename | Image version | `scripts/build-iso.sh` (reads `VERSION`) |

## Pre-1.0 Policy

While in `0.x.y`:
- **MINOR** bumps may include breaking changes (API is unstable)
- **PATCH** bumps are always backward-compatible
- Document breaking changes clearly in CHANGELOG under **Removed** or **Changed**

After `1.0.0`:
- Strict SemVer compliance
- Breaking changes require **MAJOR** bump
- Deprecation warnings for at least 1 minor version before removal

## Reference Files

- [docs/VERSIONING.md](../../docs/VERSIONING.md) — Versioning strategy
- [docs/CONTRIBUTING.md](../../docs/CONTRIBUTING.md) — Release section
- [scripts/version-bump.sh](../../scripts/version-bump.sh) — Version bump script
- [scripts/changelog-gen.sh](../../scripts/changelog-gen.sh) — Changelog generator
- [Makefile](../../Makefile) — Build and release targets
