#!/usr/bin/env bash
#
# Database backup, verification and restore for the timesheet app.
#
# Usable without make, which is not installed on the TrueNAS host. The Makefile
# targets delegate here so there is one implementation rather than two.
#
#   ./scripts/db.sh backup                 dump the database (no downtime)
#   ./scripts/db.sh verify <dump>          restore into a scratch container
#   ./scripts/db.sh restore <dump>         replace the database (destructive)
#
# The dump contains API key hashes, MCP tokens, and Google refresh tokens
# encrypted with ENCRYPTION_KEY. Treat it as a credential, and keep a copy of
# ENCRYPTION_KEY somewhere separate: without it the restored Google credentials
# cannot be decrypted and every calendar connection must be re-authorised.

set -euo pipefail

# Run from the repository root so docker compose finds its file regardless of
# where the caller happened to be.
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

BACKUP_DIR="${BACKUP_DIR:-./backups}"
PG_SERVICE="${PG_SERVICE:-postgres}"
API_SERVICE="${API_SERVICE:-api}"
PG_USER="${PG_USER:-timesheet}"
PG_DB="${PG_DB:-timesheet_v2}"
PG_IMAGE="${PG_IMAGE:-postgres:16-alpine}"

# The scratch container name must be global: an EXIT trap fires after the
# function that set it has returned, so a `local` would be out of scope by then
# - and under `set -u` that turns cleanup into an error and leaks the container.
SCRATCH_CONTAINER=""

cleanup_scratch() {
    if [ -n "$SCRATCH_CONTAINER" ]; then
        docker rm -f "$SCRATCH_CONTAINER" >/dev/null 2>&1 || true
    fi
}

usage() {
    # Print the header comment block, stopping at the first non-comment line, so
    # the usage text cannot drift from the documentation above it.
    awk 'NR>2 { if (!/^#/) exit; sub(/^# ?/, ""); print }' "${BASH_SOURCE[0]}"
    exit "${1:-0}"
}

require_file() {
    if [ -z "${1:-}" ]; then
        echo "Usage: $0 $2 <dump-file>" >&2
        exit 1
    fi
    if [ ! -f "$1" ]; then
        echo "No such file: $1" >&2
        exit 1
    fi
}

cmd_backup() {
    mkdir -p "$BACKUP_DIR"
    local file="$BACKUP_DIR/timesheet-$(date +%Y%m%d-%H%M%S).dump"

    if ! docker compose exec -T "$PG_SERVICE" \
        pg_dump -U "$PG_USER" -Fc "$PG_DB" > "$file"; then
        echo "Dump failed" >&2
        rm -f "$file"
        exit 1
    fi

    if [ ! -s "$file" ]; then
        echo "Dump is empty - is $PG_SERVICE running?" >&2
        rm -f "$file"
        exit 1
    fi

    echo "Wrote $file ($(du -h "$file" | cut -f1))"
    echo "Verify it before trusting it:  $0 verify $file"
}

# Restores into a scratch container so the dump is proved readable without
# touching the real database. An unrestored backup is only a hypothesis.
cmd_verify() {
    require_file "${1:-}" verify
    local file="$1"
    SCRATCH_CONTAINER="timesheet-verify-$$"

    trap cleanup_scratch EXIT

    echo "Starting scratch postgres..."
    docker run -d --name "$SCRATCH_CONTAINER" \
        -e POSTGRES_PASSWORD=verify \
        -e POSTGRES_USER="$PG_USER" \
        -e POSTGRES_DB="$PG_DB" \
        "$PG_IMAGE" >/dev/null

    until docker exec "$SCRATCH_CONTAINER" pg_isready -U "$PG_USER" -d "$PG_DB" >/dev/null 2>&1; do
        sleep 1
    done

    echo "Restoring $file..."
    docker exec -i "$SCRATCH_CONTAINER" pg_restore -U "$PG_USER" -d "$PG_DB" --no-owner < "$file"

    echo ""
    docker exec "$SCRATCH_CONTAINER" psql -U "$PG_USER" -d "$PG_DB" -c \
        "select relname as table, n_live_tup as rows from pg_stat_user_tables where n_live_tup > 0 order by relname;"
    echo "Backup restores cleanly."
}

cmd_restore() {
    require_file "${1:-}" restore
    local file="$1"

    echo "WARNING: This REPLACES everything in $PG_DB with $file"
    echo "Press Ctrl+C to cancel, or wait 5 seconds..."
    sleep 5

    echo "Stopping $API_SERVICE to release database connections..."
    docker compose stop "$API_SERVICE" || true

    docker compose exec -T "$PG_SERVICE" \
        pg_restore -U "$PG_USER" -d "$PG_DB" --clean --if-exists --no-owner < "$file"

    echo "Starting $API_SERVICE..."
    docker compose start "$API_SERVICE"
    echo "Restored from $file"
}

case "${1:-}" in
    backup)  shift; cmd_backup "$@" ;;
    verify)  shift; cmd_verify "$@" ;;
    restore) shift; cmd_restore "$@" ;;
    -h|--help|help) usage 0 ;;
    "")      usage 1 ;;
    *)       echo "Unknown command: $1" >&2; echo >&2; usage 1 ;;
esac
