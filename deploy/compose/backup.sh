#!/bin/sh
set -eu

: "${DATABASE_URL:?DATABASE_URL is required}"
BACKUP_DIR="${BACKUP_DIR:-/backups}"
BACKUP_KEEP_DAYS="${BACKUP_KEEP_DAYS:-7}"
BACKUP_INTERVAL_SECONDS="${BACKUP_INTERVAL_SECONDS:-86400}"

mkdir -p "$BACKUP_DIR"

while true; do
	ts="$(date -u +%Y%m%dT%H%M%SZ)"
	file="$BACKUP_DIR/sms-$ts.sql.gz"
	echo "pg_dump $file"
	pg_dump --no-owner --no-acl "$DATABASE_URL" | gzip -c > "$file.tmp"
	mv "$file.tmp" "$file"
	find "$BACKUP_DIR" -name 'sms-*.sql.gz' -mtime +"$BACKUP_KEEP_DAYS" -delete || true
	sleep "$BACKUP_INTERVAL_SECONDS"
done
