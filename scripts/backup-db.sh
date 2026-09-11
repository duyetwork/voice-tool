#!/usr/bin/env bash
#
# Backup Postgres. Chạy tay hoặc qua cron:
#
#   crontab -e
#   0 3 * * * cd /opt/voice-tool && ./scripts/backup-db.sh >> /var/log/voice-tool-backup.log 2>&1
#
# Chỉ backup Postgres, KHÔNG backup file voice: theo business rule #2, file
# voice bị xoá ngay sau khi đăng thành công — thứ nằm trên storage chỉ là voice
# đang chờ duyệt, mất thì chạy lại Bài Post là có. Postgres mới là nơi giữ toàn
# bộ dữ liệu nghiệp vụ: danh sách kênh, bài post, voice, audit log, và token
# multime + API key TTS đã mã hoá của từng người.
#
# CẢNH BÁO: bản backup nằm cùng máy không cứu được khi mất VPS. Xem phần cuối
# file để đẩy ra ngoài.
set -euo pipefail

cd "$(dirname "$0")/.."

BACKUP_DIR="${BACKUP_DIR:-/var/backups/voice-tool}"
KEEP_DAYS="${KEEP_DAYS:-7}"
COMPOSE="docker compose -f docker-compose.prod.yml"

# shellcheck disable=SC1091
[[ -f .env ]] && { set -a && source .env && set +a; }
PG_USER="${POSTGRES_USER:-voice}"
PG_DB="${POSTGRES_DB:-voice_tool}"

mkdir -p "$BACKUP_DIR"
stamp=$(date +%Y%m%d-%H%M%S)
out="$BACKUP_DIR/${PG_DB}-${stamp}.sql.gz"

# --clean --if-exists: file khôi phục được lên một DB đang có dữ liệu, không
# phải drop tay trước.
$COMPOSE exec -T postgres pg_dump -U "$PG_USER" -d "$PG_DB" --clean --if-exists |
	gzip -9 >"$out"

size=$(du -h "$out" | cut -f1)

# Bản rỗng nghĩa là pg_dump chết giữa chừng nhưng gzip vẫn tạo file — xoá ngay,
# không để một file 20 byte nằm đó giả làm bản backup.
if [[ $(stat -c %s "$out") -lt 1000 ]]; then
	rm -f "$out"
	echo "[$(date '+%F %T')] LỖI: bản dump rỗng, đã xoá. Kiểm tra: $COMPOSE logs postgres" >&2
	exit 1
fi

find "$BACKUP_DIR" -name "${PG_DB}-*.sql.gz" -mtime "+$KEEP_DAYS" -delete

echo "[$(date '+%F %T')] backup OK: $out ($size), giữ $KEEP_DAYS ngày, còn $(ls -1 "$BACKUP_DIR" | wc -l) bản"

# ---------------------------------------------------------------------------
# Khôi phục:
#
#   gunzip -c /var/backups/voice-tool/voice_tool-YYYYMMDD-HHMMSS.sql.gz \
#     | docker compose -f docker-compose.prod.yml exec -T postgres psql -U voice -d voice_tool
#
# Đẩy backup ra ngoài máy (nên làm — thêm vào cuối file này):
#
#   rclone copy "$out" remote:voice-tool-backups/    # cần cài rclone
#   aws s3 cp "$out" s3://<bucket>/voice-tool-backups/
# ---------------------------------------------------------------------------
