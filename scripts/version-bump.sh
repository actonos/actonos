#!/usr/bin/env bash
# ==============================================================================
# ActonOS — Version Bump Script
# ==============================================================================
#
# Usage:
#   bash scripts/version-bump.sh <patch|minor|major> [--dry-run]
#   bash scripts/version-bump.sh release  # Tag current version without bumping
#
# This script:
#   1. Reads the current version from VERSION file
#   2. Calculates the new version based on bump type
#   3. Updates VERSION file
#   4. Updates CHANGELOG.md (moves Unreleased to new version section)
#   5. Creates a git commit and tag
#
# ==============================================================================

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
VERSION_FILE="${ROOT_DIR}/VERSION"
CHANGELOG_FILE="${ROOT_DIR}/CHANGELOG.md"

# Flags
DRY_RUN=false
BUMP_TYPE="${1:-}"

# Parse arguments
for arg in "$@"; do
  case $arg in
    --dry-run)
      DRY_RUN=true
      ;;
  esac
done

# ==============================================================================
# Functions
# ==============================================================================

usage() {
  echo ""
  echo "Usage: $(basename "$0") <patch|minor|major|release> [--dry-run]"
  echo ""
  echo "  patch    Bump patch version  (0.1.0 → 0.1.1)"
  echo "  minor    Bump minor version  (0.1.0 → 0.2.0)"
  echo "  major    Bump major version  (0.1.0 → 1.0.0)"
  echo "  release  Tag current version without bumping"
  echo ""
  echo "Options:"
  echo "  --dry-run  Preview changes without modifying files"
  echo ""
  exit 1
}

log_info() {
  echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
  echo -e "${GREEN}[OK]${NC} $1"
}

log_warn() {
  echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
  echo -e "${RED}[ERROR]${NC} $1"
  exit 1
}

read_version() {
  if [[ ! -f "${VERSION_FILE}" ]]; then
    log_error "VERSION file not found at ${VERSION_FILE}"
  fi
  # Read first line, trim whitespace
  tr -d '[:space:]' < "${VERSION_FILE}"
}

parse_version() {
  local version="$1"
  if [[ ! "${version}" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
    log_error "Invalid version format: ${version}. Expected MAJOR.MINOR.PATCH"
  fi
  MAJOR="${BASH_REMATCH[1]}"
  MINOR="${BASH_REMATCH[2]}"
  PATCH="${BASH_REMATCH[3]}"
}

bump_version() {
  local bump_type="$1"
  case "${bump_type}" in
    patch)
      PATCH=$((PATCH + 1))
      ;;
    minor)
      MINOR=$((MINOR + 1))
      PATCH=0
      ;;
    major)
      MAJOR=$((MAJOR + 1))
      MINOR=0
      PATCH=0
      ;;
    release)
      # No version change, just tag
      ;;
    *)
      usage
      ;;
  esac
  echo "${MAJOR}.${MINOR}.${PATCH}"
}

update_version_file() {
  local new_version="$1"
  if [[ "${DRY_RUN}" == true ]]; then
    log_info "[DRY RUN] Would update VERSION to: ${new_version}"
    return
  fi
  echo "${new_version}" > "${VERSION_FILE}"
  log_success "Updated VERSION file: ${new_version}"
}

update_changelog() {
  local new_version="$1"
  local today
  today=$(date +%Y-%m-%d)

  if [[ "${DRY_RUN}" == true ]]; then
    log_info "[DRY RUN] Would update CHANGELOG.md:"
    log_info "  - Replace [Unreleased] header with [${new_version}] - ${today}"
    log_info "  - Add new empty [Unreleased] section"
    return
  fi

  if [[ ! -f "${CHANGELOG_FILE}" ]]; then
    log_warn "CHANGELOG.md not found. Skipping changelog update."
    return
  fi

  # Create a temporary file
  local tmpfile
  tmpfile=$(mktemp)

  # Replace [Unreleased] with new version section and add fresh [Unreleased]
  awk -v version="${new_version}" -v date="${today}" '
    /^## \[Unreleased\]/ {
      print "## [Unreleased]"
      print ""
      print ""
      print "## [" version "] - " date
      next
    }
    { print }
  ' "${CHANGELOG_FILE}" > "${tmpfile}"

  # Update the comparison links at the bottom
  # Add the new version comparison link
  if grep -q "\[Unreleased\]:" "${tmpfile}"; then
    sed -i "s|\[Unreleased\]:.*|[Unreleased]: https://github.com/actonos/actonos/compare/v${new_version}...HEAD\n[${new_version}]: https://github.com/actonos/actonos/releases/tag/v${new_version}|" "${tmpfile}"
  fi

  mv "${tmpfile}" "${CHANGELOG_FILE}"
  log_success "Updated CHANGELOG.md with [${new_version}] - ${today}"
}

git_commit_and_tag() {
  local new_version="$1"

  if [[ "${DRY_RUN}" == true ]]; then
    log_info "[DRY RUN] Would create git commit: chore(release): v${new_version}"
    log_info "[DRY RUN] Would create git tag: v${new_version}"
    return
  fi

  # Check for uncommitted changes (besides our modifications)
  cd "${ROOT_DIR}"

  git add VERSION CHANGELOG.md
  git commit -m "chore(release): v${new_version}"
  git tag -a "v${new_version}" -m "Release v${new_version}"

  log_success "Created commit: chore(release): v${new_version}"
  log_success "Created tag: v${new_version}"
  echo ""
  log_info "To publish this release, run:"
  echo "  git push origin main --tags"
}

# ==============================================================================
# Main
# ==============================================================================

if [[ -z "${BUMP_TYPE}" ]]; then
  usage
fi

# Filter out --dry-run from BUMP_TYPE
if [[ "${BUMP_TYPE}" == "--dry-run" ]]; then
  usage
fi

echo ""
echo "╔══════════════════════════════════════════╗"
echo "║     ActonOS Version Bump                 ║"
echo "╚══════════════════════════════════════════╝"
echo ""

# Read current version
CURRENT_VERSION=$(read_version)
parse_version "${CURRENT_VERSION}"
log_info "Current version: ${CURRENT_VERSION}"

# Calculate new version
NEW_VERSION=$(bump_version "${BUMP_TYPE}")
log_info "Bump type: ${BUMP_TYPE}"
log_info "New version: ${NEW_VERSION}"
echo ""

if [[ "${DRY_RUN}" == true ]]; then
  log_warn "DRY RUN MODE — No files will be modified"
  echo ""
fi

# Execute updates
update_version_file "${NEW_VERSION}"
update_changelog "${NEW_VERSION}"
git_commit_and_tag "${NEW_VERSION}"

echo ""
log_success "Version bump complete: ${CURRENT_VERSION} → ${NEW_VERSION}"
echo ""
