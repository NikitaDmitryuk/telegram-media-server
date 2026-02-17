#!/usr/bin/env bash
#
# merge-env.sh — merge new parameters from .env.example into .env
#
# Usage: ./scripts/merge-env.sh [.env] [.env.example]
#
# - Takes .env.example as the template (structure, comments, order)
# - Preserves all active (uncommented) values from the user's .env
# - New parameters appear commented out, exactly as in .env.example
# - User keys not present in .env.example are appended at the end
# - Creates a .env.bak backup before modifying

set -euo pipefail

ENV_FILE="${1:-.env}"
EXAMPLE_FILE="${2:-.env.example}"
BACKUP_FILE="${ENV_FILE}.bak"

if [ ! -f "$EXAMPLE_FILE" ]; then
    echo "Error: $EXAMPLE_FILE not found"
    exit 1
fi

if [ ! -f "$ENV_FILE" ]; then
    echo "No $ENV_FILE found — creating from $EXAMPLE_FILE..."
    cp "$EXAMPLE_FILE" "$ENV_FILE"
    echo "Created $ENV_FILE. Please edit it with your settings."
    exit 0
fi

cp "$ENV_FILE" "$BACKUP_FILE"

awk '
# ── First pass: collect active KEY=VALUE from .env ──
NR == FNR {
    # skip blank lines and comments
    if ($0 ~ /^[[:space:]]*$/ || $0 ~ /^[[:space:]]*#/) next
    # match KEY=...
    if ($0 ~ /^[A-Za-z_][A-Za-z0-9_]*=/) {
        key = substr($0, 1, index($0, "=") - 1)
        val = substr($0, index($0, "=") + 1)
        env[key] = val
    }
    next
}

# ── Second pass: process .env.example as template ──
{
    # Commented-out parameter: #KEY=value (no space between # and KEY)
    if ($0 ~ /^#[A-Za-z_][A-Za-z0-9_]*=/) {
        stripped = substr($0, 2)  # remove leading #
        key = substr(stripped, 1, index(stripped, "=") - 1)
        if (key in env) {
            print key "=" env[key]
            used[key] = 1
        } else {
            print
        }
        example_keys[key] = 1
        next
    }

    # Active parameter: KEY=value
    if ($0 ~ /^[A-Za-z_][A-Za-z0-9_]*=/) {
        key = substr($0, 1, index($0, "=") - 1)
        if (key in env) {
            print key "=" env[key]
            used[key] = 1
        } else {
            print
        }
        example_keys[key] = 1
        next
    }

    # Everything else (comments, section headers, blank lines)
    print
}

# ── Append user keys not found in .env.example ──
END {
    first = 1
    for (key in env) {
        if (!(key in example_keys)) {
            if (first) {
                print ""
                print "# ============================================================================="
                print "# CUSTOM SETTINGS — preserved from previous .env"
                print "# ============================================================================="
                first = 0
            }
            print key "=" env[key]
        }
    }
}
' "$ENV_FILE" "$EXAMPLE_FILE" > "${ENV_FILE}.tmp"

mv "${ENV_FILE}.tmp" "$ENV_FILE"

# Report result
if diff -q "$BACKUP_FILE" "$ENV_FILE" > /dev/null 2>&1; then
    echo "No changes — .env is already up to date."
    rm -f "$BACKUP_FILE"
else
    added=$(diff "$BACKUP_FILE" "$ENV_FILE" | grep "^> " | grep -cv "^> #" 2>/dev/null || true)
    echo "Updated $ENV_FILE (backup: $BACKUP_FILE)"
    echo "Review changes: diff $BACKUP_FILE $ENV_FILE"
fi
