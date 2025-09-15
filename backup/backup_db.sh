#!/usr/bin/env sh
set -eu

# Usage: backup_db.sh
# Requires env: DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME
#               BACKUP_S3_ENDPOINT, BACKUP_S3_REGION, BACKUP_S3_ACCESS_KEY_ID,
#               BACKUP_S3_SECRET_ACCESS_KEY, BACKUP_BUCKET

DATE=$(date -u +"%Y-%m-%dT%H-%M-%SZ")
TMP_DIR=${TMP_DIR:-/tmp}
OUT_FILE="$TMP_DIR/${DB_NAME}_${DATE}.sql.gz"

export PGPASSWORD="$DB_PASSWORD"

pg_dump -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -F p \
  | gzip -c > "$OUT_FILE"

echo "Backup created: $OUT_FILE"

# Upload to S3 (using AWS CLI-compatible tool 'aws' if available)
if command -v aws >/dev/null 2>&1; then
  aws configure set aws_access_key_id "$BACKUP_S3_ACCESS_KEY_ID"
  aws configure set aws_secret_access_key "$BACKUP_S3_SECRET_ACCESS_KEY"
  aws configure set region "$BACKUP_S3_REGION"
  aws s3api put-object --endpoint-url "$BACKUP_S3_ENDPOINT" \
    --bucket "$BACKUP_BUCKET" --key "db/$DB_NAME/$DATE.sql.gz" --body "$OUT_FILE"
  echo "Uploaded to s3://$BACKUP_BUCKET/db/$DB_NAME/$DATE.sql.gz"
else
  echo "aws CLI not found. Please install it to upload backups to S3." >&2
fi

rm -f "$OUT_FILE"
echo "Done."


