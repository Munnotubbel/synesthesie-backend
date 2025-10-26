#!/usr/bin/env bash
set -euo pipefail

# Synesthesie Database Backup Script
# Requires env: DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME
#               BACKUP_S3_ENDPOINT, BACKUP_S3_REGION, BACKUP_S3_ACCESS_KEY_ID,
#               BACKUP_S3_SECRET_ACCESS_KEY, BACKUP_BUCKET

echo "===================================="
echo "Synesthesie Database Backup"
echo "Started at: $(date)"
echo "===================================="

# Check required environment variables
REQUIRED_VARS="DB_HOST DB_PORT DB_USER DB_PASSWORD DB_NAME BACKUP_S3_ENDPOINT BACKUP_S3_REGION BACKUP_S3_ACCESS_KEY_ID BACKUP_S3_SECRET_ACCESS_KEY BACKUP_BUCKET"
for var in $REQUIRED_VARS; do
  if [ -z "${!var:-}" ]; then
    echo "ERROR: Required environment variable $var is not set!" >&2
    exit 1
  fi
done

# Check if required tools are installed
if ! command -v pg_dump >/dev/null 2>&1; then
  echo "ERROR: pg_dump not found. Please install postgresql-client." >&2
  exit 1
fi

if ! command -v aws >/dev/null 2>&1; then
  echo "ERROR: aws CLI not found. Please install awscli." >&2
  exit 1
fi

# Generate filename
DATE=$(date -u +"%Y-%m-%dT%H-%M-%SZ")
TMP_DIR=${TMP_DIR:-/tmp}
OUT_FILE="$TMP_DIR/${DB_NAME}_${DATE}.sql.gz"

echo "Database: $DB_NAME"
echo "Host: $DB_HOST:$DB_PORT"
echo "Backup file: $OUT_FILE"

# Set PostgreSQL password
export PGPASSWORD="$DB_PASSWORD"

# Create database dump
echo "Creating database dump..."
if pg_dump -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -F p | gzip -c > "$OUT_FILE"; then
  FILE_SIZE=$(du -h "$OUT_FILE" | cut -f1)
  echo "✓ Backup created successfully ($FILE_SIZE)"
else
  echo "ERROR: pg_dump failed!" >&2
  rm -f "$OUT_FILE"
  exit 1
fi

# Upload to S3
echo "Uploading to S3..."
S3_KEY="db/$DB_NAME/$DATE.sql.gz"
S3_PATH="s3://$BACKUP_BUCKET/$S3_KEY"

export AWS_ACCESS_KEY_ID="$BACKUP_S3_ACCESS_KEY_ID"
export AWS_SECRET_ACCESS_KEY="$BACKUP_S3_SECRET_ACCESS_KEY"
export AWS_DEFAULT_REGION="$BACKUP_S3_REGION"

if aws s3api put-object \
  --endpoint-url "$BACKUP_S3_ENDPOINT" \
  --bucket "$BACKUP_BUCKET" \
  --key "$S3_KEY" \
  --body "$OUT_FILE" \
  --region "$BACKUP_S3_REGION" \
  >/dev/null 2>&1; then
  echo "✓ Uploaded to $S3_PATH"
else
  echo "ERROR: S3 upload failed!" >&2
  echo "Keeping local backup at: $OUT_FILE" >&2
  exit 1
fi

# Cleanup
rm -f "$OUT_FILE"
echo "✓ Local backup file removed"

echo "===================================="
echo "Backup completed successfully!"
echo "Finished at: $(date)"
echo "===================================="


