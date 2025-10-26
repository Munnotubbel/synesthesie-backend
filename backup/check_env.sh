#!/usr/bin/env bash
# Check if all required environment variables are set for backup

echo "=== Backup Environment Check ==="
echo ""

# Load .env.backup if exists, otherwise .env
if [ -f ".env.backup" ]; then
  echo "Loading: .env.backup"
  set -a
  source .env.backup
  set +a
elif [ -f ".env" ]; then
  echo "Loading: .env"
  echo "WARNING: Consider creating .env.backup with DB_HOST=localhost"
  set -a
  source .env
  set +a
else
  echo "ERROR: No .env or .env.backup found!"
  exit 1
fi

echo ""
echo "=== Required Variables ==="

REQUIRED_VARS="DB_HOST DB_PORT DB_USER DB_PASSWORD DB_NAME BACKUP_S3_ENDPOINT BACKUP_S3_REGION BACKUP_S3_ACCESS_KEY_ID BACKUP_S3_SECRET_ACCESS_KEY BACKUP_BUCKET"

ALL_OK=true
for var in $REQUIRED_VARS; do
  if [ -z "${!var:-}" ]; then
    echo "❌ $var: NOT SET"
    ALL_OK=false
  else
    # Mask sensitive values
    if [[ "$var" == *"PASSWORD"* ]] || [[ "$var" == *"SECRET"* ]] || [[ "$var" == *"KEY"* ]]; then
      echo "✓ $var: ***MASKED*** (${#!var} chars)"
    else
      echo "✓ $var: ${!var}"
    fi
  fi
done

echo ""
if [ "$ALL_OK" = true ]; then
  echo "✅ All required variables are set!"
  echo ""
  echo "=== Tool Check ==="

  if command -v pg_dump >/dev/null 2>&1; then
    echo "✓ pg_dump: $(which pg_dump)"
  else
    echo "❌ pg_dump: NOT FOUND (install: sudo apt install postgresql-client)"
  fi

  if command -v aws >/dev/null 2>&1; then
    echo "✓ aws: $(which aws)"
  else
    echo "❌ aws: NOT FOUND (install: pip3 install awscli)"
  fi

  echo ""
  echo "=== Database Connection Test ==="
  export PGPASSWORD="$DB_PASSWORD"
  if pg_dump -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" --schema-only -t nonexistent_table 2>&1 | grep -q "does not exist"; then
    echo "✅ Database connection successful!"
  else
    echo "❌ Database connection failed!"
    echo "   Check: DB_HOST=$DB_HOST, DB_PORT=$DB_PORT"
    echo "   Hint: For Docker, use DB_HOST=localhost and the mapped port (e.g. 5433)"
  fi

  exit 0
else
  echo "❌ Some required variables are missing!"
  exit 1
fi




