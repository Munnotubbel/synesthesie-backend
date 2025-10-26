#!/bin/bash
set -euo pipefail

# Synesthesie Backup Wrapper Script
# Loads .env and executes backup_db.sh
# Usage: ./run_backup.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "Project directory: $PROJECT_DIR"

# Try .env.backup first (for host-side backups with localhost:5433), fallback to .env
if [ -f "$PROJECT_DIR/.env.backup" ]; then
  echo "Loading environment from: $PROJECT_DIR/.env.backup"
  ENV_FILE="$PROJECT_DIR/.env.backup"
elif [ -f "$PROJECT_DIR/.env" ]; then
  echo "Loading environment from: $PROJECT_DIR/.env"
  echo "WARNING: Using .env - consider creating .env.backup with DB_HOST=localhost and DB_PORT=5433"
  ENV_FILE="$PROJECT_DIR/.env"
else
  echo "ERROR: Neither .env.backup nor .env file found in $PROJECT_DIR" >&2
  exit 1
fi

# Load .env and export all variables
set -a
source "$ENV_FILE"
set +a

echo "Environment loaded successfully"
echo "Executing backup script..."

# Execute backup
"$SCRIPT_DIR/backup_db.sh"

