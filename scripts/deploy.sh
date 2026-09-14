#!/usr/bin/env bash
#
# Deploy trên VPS. Chạy trong /opt/voice-tool sau khi image đã được đẩy sang
# (scripts/push-images.sh).
#
#   ./scripts/deploy.sh
#
# Thứ tự có chủ ý: backup TRƯỚC khi migration chạm vào schema, kiểm tra cấu
# hình TRƯỚC khi khởi động (sai .env mà vẫn `up -d` là hệ thống nửa sống nửa
# chết), và KHÔNG bao giờ truyền --build (xem push-images.sh).
set -euo pipefail

cd "$(dirname "$0")/.."
COMPOSE="docker compose -f docker-compose.prod.yml"

log() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }
fail() { printf '\n\033[1;31m!!  %s\033[0m\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# 1. Kiểm tra cấu hình trước khi đụng vào gì
# ---------------------------------------------------------------------------
log "Kiểm tra .env"
[[ -f .env ]] || fail ".env không tồn tại. cp .env.prod.example .env && vi .env"

# shellcheck disable=SC1091
set -a && source .env && set +a

for var in SITE_DOMAIN POSTGRES_PASSWORD DATABASE_URL JWT_SECRET TOKEN_ENCRYPTION_KEY \
	BOOTSTRAP_ADMIN_EMAIL S3_ACCESS_KEY S3_SECRET_KEY; do
	[[ -n "${!var:-}" ]] || fail "$var đang trống trong .env"
done

# Mật khẩu Postgres nằm ở 2 chỗ (biến của container và chuỗi kết nối). Lệch
# nhau là api không vào được DB, mà lỗi hiện ra lại là "connection refused" khó
# đoán — nên chặn ngay ở đây.
[[ "$DATABASE_URL" == *"$POSTGRES_PASSWORD"* ]] ||
	fail "DATABASE_URL không chứa POSTGRES_PASSWORD — sửa cho khớp rồi chạy lại"

# TOKEN_ENCRYPTION_KEY phải là base64 của đúng 32 byte, sai là api fail-fast.
key_bytes=$(printf '%s' "$TOKEN_ENCRYPTION_KEY" | base64 -d 2>/dev/null | wc -c || echo 0)
[[ "$key_bytes" == "32" ]] ||
	fail "TOKEN_ENCRYPTION_KEY phải là base64 của 32 byte (đang là $key_bytes byte). Sinh: head -c 32 /dev/urandom | base64"

$COMPOSE config -q || fail "docker-compose.prod.yml không hợp lệ"

# ---------------------------------------------------------------------------
# 2. Backup trước khi migration chạy
# ---------------------------------------------------------------------------
if $COMPOSE ps --status running --services 2>/dev/null | grep -q '^postgres$'; then
	log "Backup database trước khi deploy"
	./scripts/backup-db.sh
else
	log "Postgres chưa chạy (lần deploy đầu) — bỏ qua backup"
fi

# ---------------------------------------------------------------------------
# 3. Khởi động. migrate chạy trước, api/worker/scheduler chờ nó xong.
# ---------------------------------------------------------------------------
log "Khởi động (không build — image được đẩy từ máy dev)"
$COMPOSE up -d --remove-orphans

log "Chờ các service healthy"
for _ in $(seq 1 30); do
	unhealthy=$($COMPOSE ps --format '{{.Service}} {{.Health}}' 2>/dev/null |
		awk '$2 != "" && $2 != "healthy" {print $1}' | tr '\n' ' ')
	[[ -z "$unhealthy" ]] && break
	sleep 5
done
[[ -z "${unhealthy:-}" ]] || fail "Service chưa healthy: $unhealthy — xem: $COMPOSE logs"

# ---------------------------------------------------------------------------
# 4. Kiểm tra sau khi lên
# ---------------------------------------------------------------------------
log "Kiểm tra"

# Scheduler phải đúng 1 process: 2 process = mỗi kênh bị quét 2 lần, tốn gấp
# đôi quota nền tảng và chi phí AI.
sched=$($COMPOSE ps --status running --format '{{.Service}}' | grep -c '^scheduler$' || true)
[[ "$sched" == "1" ]] || fail "Có $sched scheduler đang chạy — phải đúng 1"

# Migration: dirty=true nghĩa là một migration chết giữa chừng, schema đang ở
# trạng thái nửa vời và phải sửa tay trước khi chạy tiếp.
dirty=$($COMPOSE exec -T postgres psql -U "${POSTGRES_USER:-voice}" -d "${POSTGRES_DB:-voice_tool}" \
	-tAc "select dirty from schema_migrations" 2>/dev/null || echo "?")
[[ "$dirty" != "t" ]] || fail "schema_migrations.dirty = true — migration hỏng giữa chừng, phải sửa tay"

version=$($COMPOSE exec -T postgres psql -U "${POSTGRES_USER:-voice}" -d "${POSTGRES_DB:-voice_tool}" \
	-tAc "select version from schema_migrations" 2>/dev/null || echo "?")

# SITE_DOMAIN có thể đã kèm scheme — khi chưa trỏ tên miền, người ta điền tạm
# "http://<ip>". Ghép thêm https:// nữa thì ra "https://http://<ip>": URL đó
# không bao giờ gọi được, nên bước kiểm tra luôn rơi xuống nhánh dự phòng và
# báo "chưa vào được qua tên miền" dù web vẫn chạy hoàn toàn bình thường.
case "$SITE_DOMAIN" in
http://* | https://*) SITE_URL="$SITE_DOMAIN" ;;
*) SITE_URL="https://$SITE_DOMAIN" ;;
esac

sleep 3
if curl -sfk "${SITE_URL}/healthz" >/dev/null 2>&1; then
	health="${SITE_URL}/healthz OK"
elif $COMPOSE exec -T api wget -qO- http://localhost:8080/healthz >/dev/null 2>&1; then
	health="api OK (chưa vào được qua tên miền — kiểm tra DNS đã trỏ về máy này chưa)"
else
	fail "api không trả lời /healthz — xem: $COMPOSE logs api"
fi

# Ảnh cũ sau vài lần deploy chiếm vài GB trên đĩa 30GB.
docker image prune -f >/dev/null

cat <<EOF

  Migration : version $version
  Health    : $health
  Đĩa       : $(df -h / | awk 'NR==2 {print $4}') trống
  RAM       : $(free -h | awk '/^Mem:/ {printf "%s dùng / %s", $3, $2}')  |  swap: $(free -h | awk '/^Swap:/ {print $3}')

  Web: ${SITE_URL}

EOF
