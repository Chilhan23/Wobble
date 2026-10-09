#!/bin/bash
set -euo pipefail
export PATH="$HOME/go/bin:$PATH"


# Load .env
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[ -f "$ROOT/.env" ] && set -a && source "$ROOT/.env" && set +a

DIR="$ROOT/migrations"
DB="$DATABASE_URL"

case "${1:-}" in
  up)
    migrate -path "$DIR" -database "$DB" up ${2:-}
    echo "✅ Migrations applied!"
    ;;
  down)
    count="${2:-1}"
    if [ "$count" = "all" ]; then
      read -p "⚠️  Rollback ALL migrations? Type 'yes': " c
      [ "$c" = "yes" ] && migrate -path "$DIR" -database "$DB" down -all || echo "❌ Cancelled."
    else
      read -p "⚠️  Rollback $count migration(s)? (y/N): " c
      [[ "$c" =~ ^[yY]$ ]] && echo "y" | migrate -path "$DIR" -database "$DB" down "$count" || echo "❌ Cancelled."
    fi
    ;;
  create)
    [ -z "${2:-}" ] && echo "Usage: $0 create <name>" && exit 1
    migrate create -ext sql -dir "$DIR" -seq "$2"
    echo "✅ Migration created!"
    ;;
  redo)
    read -p "🔄 Redo last migration? (y/N): " c
    [[ "$c" =~ ^[yY]$ ]] && echo "y" | migrate -path "$DIR" -database "$DB" down 1 && migrate -path "$DIR" -database "$DB" up 1 || echo "❌ Cancelled."
    ;;
  status)
    migrate -path "$DIR" -database "$DB" version
    ;;
  force)
    [ -z "${2:-}" ] && echo "Usage: $0 force <version>" && exit 1
    migrate -path "$DIR" -database "$DB" force "$2"
    ;;
  *)
    echo "Usage: $0 {up|down|create|redo|status|force} [args]"
    ;;
esac
