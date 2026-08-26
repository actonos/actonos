#!/usr/bin/env bash
# ==============================================================================
# ActonOS — Changelog Generator
# ==============================================================================
#
# Generates changelog entries from Conventional Commits since the last git tag.
# Prints Keep a Changelog sections to stdout and writes them under
# ## [Unreleased] in CHANGELOG.md (replacing the previous Unreleased body).
#
# Usage:
#   bash scripts/changelog-gen.sh                 # Since last tag; update CHANGELOG.md
#   bash scripts/changelog-gen.sh v0.1.0          # Since specific tag
#   bash scripts/changelog-gen.sh --all           # All commits
#   bash scripts/changelog-gen.sh --stdout-only   # Print only; do not edit CHANGELOG.md
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

STDOUT_ONLY=false
SINCE_TAG=""
for arg in "$@"; do
  case "${arg}" in
    --stdout-only) STDOUT_ONLY=true ;;
    --all) SINCE_TAG="--all" ;;
    -*)
      echo -e "${YELLOW}[WARN]${NC} Unknown flag ${arg}" >&2
      ;;
    *) SINCE_TAG="${arg}" ;;
  esac
done

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

  # Extract type and message from conventional commit format.
  # Keep the pattern in a variable: unquoted `)` inside [[ =~ ]] is a syntax error.
  commit_re='^(feat|fix|docs|style|refactor|perf|test|build|ci|chore)(\(([^)]+)\))?!?: (.+)$'
  if [[ "${line}" =~ $commit_re ]]; then
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
done < <(git log ${RANGE} --pretty=tformat:"%s" --no-merges 2>/dev/null)

# ==============================================================================
# Output
# ==============================================================================

BODY=""
append_section() {
  local title="$1"
  shift
  if [[ $# -eq 0 ]]; then
    return
  fi
  BODY+="### ${title}"$'\n'
  local line
  for line in "$@"; do
    BODY+="${line}"$'\n'
  done
  BODY+=$'\n'
}

if [[ ${#ADDED[@]} -gt 0 ]]; then append_section "Added" "${ADDED[@]}"; fi
if [[ ${#CHANGED[@]} -gt 0 ]]; then append_section "Changed" "${CHANGED[@]}"; fi
if [[ ${#DEPRECATED[@]} -gt 0 ]]; then append_section "Deprecated" "${DEPRECATED[@]}"; fi
if [[ ${#REMOVED[@]} -gt 0 ]]; then append_section "Removed" "${REMOVED[@]}"; fi
if [[ ${#FIXED[@]} -gt 0 ]]; then append_section "Fixed" "${FIXED[@]}"; fi
if [[ ${#SECURITY[@]} -gt 0 ]]; then append_section "Security" "${SECURITY[@]}"; fi
if [[ ${#PERFORMANCE[@]} -gt 0 ]]; then append_section "Performance" "${PERFORMANCE[@]}"; fi
if [[ ${#OTHER[@]} -gt 0 ]]; then append_section "Other" "${OTHER[@]}"; fi

if [[ -z "${BODY}" ]]; then
  echo -e "${YELLOW}[WARN]${NC} No commits found in the specified range." >&2
  exit 0
fi

echo ""
printf '%s' "${BODY}"

if [[ "${STDOUT_ONLY}" == true ]]; then
  exit 0
fi

CHANGELOG_FILE="${ROOT_DIR}/CHANGELOG.md"
if [[ ! -f "${CHANGELOG_FILE}" ]]; then
  echo -e "${YELLOW}[WARN]${NC} CHANGELOG.md not found; skipped file update." >&2
  exit 0
fi

genfile=$(mktemp)
printf '%s' "${BODY}" > "${genfile}"
tmpfile=$(mktemp)
awk -v genfile="${genfile}" '
  BEGIN {
    while ((getline line < genfile) > 0) {
      gen = gen line "\n"
    }
    close(genfile)
  }
  /^## \[Unreleased\]/ {
    print
    print ""
    printf "%s", gen
    skip = 1
    next
  }
  skip && /^## \[/ {
    skip = 0
    print
    next
  }
  skip { next }
  { print }
' "${CHANGELOG_FILE}" > "${tmpfile}"
mv "${tmpfile}" "${CHANGELOG_FILE}"
rm -f "${genfile}"
echo -e "${BLUE}[INFO]${NC} Wrote Unreleased section to CHANGELOG.md" >&2
