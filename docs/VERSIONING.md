# Versioning Strategy

> How ActonOS versions, releases, and changelog are managed.

---

## Semantic Versioning

ActonOS follows [Semantic Versioning 2.0.0](https://semver.org/):

```
MAJOR.MINOR.PATCH
```

| Component | Increment When | Example |
|:---|:---|:---|
| **MAJOR** | Incompatible API/schema changes, breaking agent manifest format | `0.x.x` → `1.0.0` |
| **MINOR** | New features, backward-compatible additions | `0.1.x` → `0.2.0` |
| **PATCH** | Bug fixes, security patches, documentation | `0.1.0` → `0.1.1` |

### Pre-1.0 Convention

While in `0.x.y` development:
- **MINOR** bumps may include breaking changes
- **PATCH** bumps are always backward-compatible
- The API is not considered stable until `1.0.0`

---

## Version Source of Truth

The single source of truth for the current version is the [`VERSION`](../VERSION) file in the repository root:

```
0.1.0
```

This file is read by:
- `Makefile` — passes version via `-ldflags` to the Go binary
- `scripts/version-bump.sh` — reads and updates the version
- `scripts/changelog-gen.sh` — determines the current release section
- CI/CD pipelines — tags Docker images and GitHub releases

---

## Version Bump Workflow

### Using Make Targets

```bash
# Bump patch version (0.1.0 → 0.1.1)
make bump-patch

# Bump minor version (0.1.0 → 0.2.0)
make bump-minor

# Bump major version (0.1.0 → 1.0.0)
make bump-major
```

### What the Script Does

The `scripts/version-bump.sh` script performs these steps:

1. **Reads** the current version from `VERSION`
2. **Calculates** the new version based on the bump type
3. **Updates** the `VERSION` file
4. **Updates** `CHANGELOG.md`:
   - Moves `[Unreleased]` entries under a new dated `[X.Y.Z]` section
   - Adds a fresh empty `[Unreleased]` section
   - Updates the comparison links at the bottom
5. **Creates** a git commit: `chore(release): vX.Y.Z`
6. **Creates** a git tag: `vX.Y.Z`

### Dry Run

Preview changes without modifying any files:

```bash
bash scripts/version-bump.sh patch --dry-run
```

---

## Changelog Management

### Format

The [CHANGELOG.md](../CHANGELOG.md) follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/):

```markdown
## [Unreleased]

### Added
- New features

### Changed
- Changes to existing features

### Deprecated
- Features to be removed in the future

### Removed
- Removed features

### Fixed
- Bug fixes

### Security
- Security-related fixes
```

### Adding Entries

When making changes, add an entry to the `[Unreleased]` section of `CHANGELOG.md` under the appropriate category:

```markdown
## [Unreleased]

### Added
- Agent swarm delegation with configurable timeout (#42)

### Fixed
- FTS5 index corruption on concurrent writes (#38)
```

### Generating Changelog from Commits

For convenience, use the changelog generation script to auto-populate entries from Conventional Commits since the last tag:

```bash
bash scripts/changelog-gen.sh
```

This script groups commits by type (`feat` → Added, `fix` → Fixed, etc.) and outputs formatted changelog entries to stdout. You can then review and paste them into `CHANGELOG.md`.

---

## Release Branch Workflow

### Standard Release

```mermaid
gitgraph
    commit id: "feat: A"
    commit id: "fix: B"
    commit id: "feat: C"
    branch release/v0.2.0
    commit id: "chore(release): v0.2.0"
    checkout main
    merge release/v0.2.0 tag: "v0.2.0"
    commit id: "feat: D"
```

1. All development happens on `main` or feature branches
2. When ready to release, run `make bump-minor` (or patch/major)
3. The script creates the release commit and tag on `main`
4. Push the tag: `git push origin main --tags`
5. CI/CD builds and publishes artifacts (Docker image, ISO, GitHub Release)

### Hotfix Release

```mermaid
gitgraph
    commit id: "v0.2.0" tag: "v0.2.0"
    commit id: "feat: E"
    branch hotfix/v0.2.1
    checkout hotfix/v0.2.1
    commit id: "fix: critical bug"
    commit id: "chore(release): v0.2.1"
    checkout main
    merge hotfix/v0.2.1 tag: "v0.2.1"
```

1. Create a `hotfix/vX.Y.Z` branch from the tagged release
2. Apply the fix
3. Run `make bump-patch`
4. Merge back to `main`
5. Push with tags

---

## Build Version Metadata

The Go binary embeds version information via linker flags:

```go
// cmd/actond/main.go
var (
    Version   string // e.g., "0.1.0"
    GitCommit string // e.g., "a1b2c3d" or "a1b2c3d-dirty"
    BuildTime string // e.g., "2026-08-16T12:00:00Z"
)
```

These are set at build time by the Makefile:

```makefile
LDFLAGS := -s -w \
    -X main.Version=$(VERSION) \
    -X main.GitCommit=$(GIT_COMMIT)$(GIT_DIRTY) \
    -X main.BuildTime=$(BUILD_TIME)
```

Access via the health endpoint:
```bash
curl http://localhost:8080/api/health
# { "version": "0.1.0", "git_commit": "a1b2c3d", ... }
```

---

## Docker Image Tags

| Tag | Description |
|:---|:---|
| `actonos/actonos:latest` | Latest stable release |
| `actonos/actonos:0.2.0` | Specific release version |
| `actonos/actonos:main` | Latest build from main branch (unstable) |

---

## OTA Update Versioning

The bare-metal OTA system uses the same SemVer scheme:

```
/data/releases/
├── v0.1.0/actond
├── v0.1.1/actond    ← previous
└── v0.2.0/actond    ← current (symlinked from /data/bin/actond)
```

`OTAEngine.Rollback()` restores the persisted previous binary from `/data/releases/state.json`.
