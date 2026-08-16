#!/usr/bin/env bash
# ==============================================================================
# ActonOS — Changelog Generator
# ==============================================================================
#
# Generates changelog entries from Conventional Commits since the last git tag.
# Output is formatted for Keep a Changelog and printed to stdout.
#
# Usage:
#   bash scripts/changelog-gen.sh              # Since last tag
#   bash scripts/changelog-gen.sh v0.1.0       # Since specific tag
#   bash scripts/changelog-gen.sh --all        # All commits
#
# ==============================================================================

set -euo pipefail

# Colors
BLUE='\033[0;34m'
YELLOW='\033[0;33m'
NC='\033[0m'

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${ROOT_DIR}"

# ==============================================================================
# Determine the range
# ==============================================================================

SINCE_TAG="${1:-}"

if [[ "${SINCE_TAG}" == "--all" ]]; then
  RANGE=""
  echo -e "${BLUE}[INFO]${NC} Generating changelog from all commits" >&2
elif [[ -n "${SINCE_TAG}" ]]; then
  RANGE="${SINCE_TAG}..HEAD"
  echo -e "${BLUE}[INFO]${NC} Generating changelog since ${SINCE_TAG}" >&2
else
  # Find the last tag
  LAST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
  if [[ -n "${LAST_TAG}" ]]; then
    RANGE="${LAST_TAG}..HEAD"
    echo -e "${BLUE}[INFO]${NC} Generating changelog since ${LAST_TAG}" >&2
  else
    RANGE=""
    echo -e "${YELLOW}[WARN]${NC} No previous tags found. Using all commits." >&2
  fi
fi

# ==============================================================================
# Collect commits by type
# ==============================================================================

declare -a ADDED=()
declare -a CHANGED=()
declare -a DEPRECATED=()
declare -a REMOVED=()
declare -a FIXED=()
declare -a SECURITY=()
declare -a PERFORMANCE=()
declare -a OTHER=()

while IFS= read -r line; do
  [[ -z "${line}" ]] && continue

  # Extract type and message from conventional commit format
  if [[ "${line}" =~ ^(feat|fix|docs|style|refactor|perf|test|build|ci|chore)(\(([^)]+)\))?!?:\ (.+)$ ]]; then
    type="${BASH_REMATCH[1]}"
    scope="${BASH_REMATCH[3]:-}"
    message="${BASH_REMATCH[4]}"

    # Format the entry
    if [[ -n "${scope}" ]]; then
      entry="**${scope}**: ${message}"
    else
      entry="${message}"
    fi

    # Categorize
    case "${type}" in
      feat)
        ADDED+=("- ${entry}")
        ;;
      fix)
        FIXED+=("- ${entry}")
        ;;
      perf)
        PERFORMANCE+=("- ${entry}")
        ;;
      docs|style|refactor|test|build|ci|chore)
        CHANGED+=("- ${entry}")
        ;;
    esac

    # Check for breaking change indicator
    if [[ "${line}" =~ !: ]]; then
      REMOVED+=("- ⚠️ BREAKING: ${entry}")
    fi
  else
    # Non-conventional commit
    OTHER+=("- ${line}")
  fi
done < <(git log ${RANGE} --pretty=format:"%s" --no-merges 2>/dev/null)

# ==============================================================================
# Output
# ==============================================================================

echo ""

has_content=false

if [[ ${#ADDED[@]} -gt 0 ]]; then
  echo "### Added"
  printf '%s\n' "${ADDED[@]}"
  echo ""
  has_content=true
fi

if [[ ${#CHANGED[@]} -gt 0 ]]; then
  echo "### Changed"
  printf '%s\n' "${CHANGED[@]}"
  echo ""
  has_content=true
fi

if [[ ${#DEPRECATED[@]} -gt 0 ]]; then
  echo "### Deprecated"
  printf '%s\n' "${DEPRECATED[@]}"
  echo ""
  has_content=true
fi

if [[ ${#REMOVED[@]} -gt 0 ]]; then
  echo "### Removed"
  printf '%s\n' "${REMOVED[@]}"
  echo ""
  has_content=true
fi

if [[ ${#FIXED[@]} -gt 0 ]]; then
  echo "### Fixed"
  printf '%s\n' "${FIXED[@]}"
  echo ""
  has_content=true
fi

if [[ ${#SECURITY[@]} -gt 0 ]]; then
  echo "### Security"
  printf '%s\n' "${SECURITY[@]}"
  echo ""
  has_content=true
fi

if [[ ${#PERFORMANCE[@]} -gt 0 ]]; then
  echo "### Performance"
  printf '%s\n' "${PERFORMANCE[@]}"
  echo ""
  has_content=true
fi

if [[ ${#OTHER[@]} -gt 0 ]]; then
  echo "### Other"
  printf '%s\n' "${OTHER[@]}"
  echo ""
  has_content=true
fi

if [[ "${has_content}" == false ]]; then
  echo -e "${YELLOW}[WARN]${NC} No commits found in the specified range." >&2
fi
