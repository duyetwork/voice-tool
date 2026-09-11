#!/usr/bin/env bash
#
# Build image ở MÁY BẠN rồi đẩy thẳng sang VPS qua SSH.
#
#   ./scripts/push-images.sh deploy@103.121.89.215
#   IMAGE_TAG=$(git rev-parse --short HEAD) ./scripts/push-images.sh deploy@1.2.3.4
#
# Vì sao không build trên VPS: `npm run build` của Next cần ~1.5-2GB RAM. Máy
# 2GB đang chạy Postgres + Redis + MinIO + worker thì không còn chừng đó, OOM
# killer sẽ bắn tiến trình build hoặc bắn nhầm Postgres. Build ở máy bạn (RAM
# thoải mái) rồi chuyển image sang là xong — không cần registry.
#
# Ảnh nén lại còn ~150-250MB, đường truyền chậm thì mất vài phút. Khi deploy
# thường xuyên thì nên chuyển sang registry (GHCR/ECR) + `docker compose pull`.
set -euo pipefail

TARGET="${1:-}"
SSH_PORT="${SSH_PORT:-22}"
IMAGE_TAG="${IMAGE_TAG:-prod}"
REMOTE_DIR="${REMOTE_DIR:-/opt/voice-tool}"

if [[ -z "$TARGET" ]]; then
	echo "Dùng: $0 user@host   (ví dụ: $0 deploy@103.121.89.215)" >&2
	exit 1
fi

cd "$(dirname "$0")/.."

IMAGES=(
	"voice-tool-api:${IMAGE_TAG}"
	"voice-tool-worker:${IMAGE_TAG}"
	"voice-tool-scheduler:${IMAGE_TAG}"
	"voice-tool-frontend:${IMAGE_TAG}"
)

log() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }

log "Build 4 image (tag: ${IMAGE_TAG})"
IMAGE_TAG="$IMAGE_TAG" docker compose -f docker-compose.prod.yml build

log "Kiểm tra image đã có"
for img in "${IMAGES[@]}"; do
	docker image inspect "$img" >/dev/null
	printf '    %-40s %s\n' "$img" "$(docker image inspect "$img" --format '{{.Size}}' | numfmt --to=iec 2>/dev/null || echo '')"
done

# gzip -1: nén nhanh, tỉ lệ đủ tốt cho image (phần lớn là binary đã nén sẵn).
# Nén mạnh hơn chỉ tốn CPU mà không tiết kiệm thêm bao nhiêu.
log "Đẩy sang ${TARGET} (nén trên đường truyền)"
docker save "${IMAGES[@]}" |
	gzip -1 |
	ssh -p "$SSH_PORT" -o ServerAliveInterval=30 "$TARGET" 'gunzip | docker load'

log "Đồng bộ file cấu hình (compose, Caddyfile, migrations, scripts)"
# .env KHÔNG nằm trong danh sách: nó chứa secret và chỉ tồn tại trên máy chủ.
rsync -az --delete -e "ssh -p $SSH_PORT" \
	--include='backend/' --include='backend/migrations/***' \
	--include='scripts/***' \
	--include='docker-compose.prod.yml' --include='Caddyfile' \
	--include='.env.prod.example' \
	--exclude='*' \
	./ "${TARGET}:${REMOTE_DIR}/" 2>/dev/null ||
	echo "  (không có rsync — dùng git pull trên VPS thay thế)"

cat <<EOF

Xong. Trên VPS chạy:

    ssh -p $SSH_PORT $TARGET 'cd $REMOTE_DIR && ./scripts/deploy.sh'

EOF
