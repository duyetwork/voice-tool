#!/usr/bin/env bash
#
# Gia cố máy chủ trước khi deploy — chạy MỘT LẦN, bằng root, trên VPS.
#
#   ssh root@103.121.89.215
#   curl -fsSL https://raw.githubusercontent.com/<repo>/main/scripts/bootstrap-vps.sh -o bootstrap.sh
#   # hoặc: scp scripts/bootstrap-vps.sh root@103.121.89.215:
#   bash bootstrap-vps.sh "ssh-ed25519 AAAA... ten-may-cua-ban"
#
# Tham số 1 (khuyến nghị): SSH public key của bạn. Có key thì script tắt đăng
# nhập bằng mật khẩu và cấm root đăng nhập trực tiếp. KHÔNG có key thì hai bước
# đó bị bỏ qua — vì tắt mật khẩu khi chưa có key là tự khoá mình ngoài cửa.
#
# Việc script làm, theo thứ tự:
#   1. swap 4GB      — máy 2GB không có swap thì ffmpeg xử lý video dài là chết
#   2. cập nhật hệ thống + bật vá bảo mật tự động
#   3. user thường có sudo (không chạy mọi thứ bằng root)
#   4. khoá SSH (chỉ khi có key)
#   5. tường lửa: chỉ 22/80/443, cho cả IPv4 lẫn IPv6
#   6. fail2ban
#   7. Docker + giới hạn log toàn cục
#
# Chạy lại được nhiều lần: bước nào đã xong thì bỏ qua.
set -euo pipefail

SSH_PUBKEY="${1:-}"
DEPLOY_USER="${DEPLOY_USER:-deploy}"
SWAP_SIZE="${SWAP_SIZE:-4G}"
TIMEZONE="${TIMEZONE:-Asia/Ho_Chi_Minh}"

# Cổng SSH thật của máy. Tự dò từ sshd đang chạy thay vì mặc định 22: máy đổi
# cổng mà ufw chỉ mở 22 là khoá luôn chính phiên đang gõ lệnh này.
detect_ssh_port() {
	local port
	port=$(sshd -T 2>/dev/null | awk '/^port /{print $2; exit}')
	[[ -z "$port" ]] && port=$(ss -tlnp 2>/dev/null | awk '/sshd/{split($4,a,":"); print a[length(a)]; exit}')
	echo "${port:-22}"
}
SSH_PORT="${SSH_PORT:-$(detect_ssh_port)}"

log() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }
warn() { printf '\033[1;33m!!  %s\033[0m\n' "$*"; }

[[ $EUID -eq 0 ]] || { echo "Phải chạy bằng root"; exit 1; }

# ---------------------------------------------------------------------------
# 1. Swap
#
# Máy 2GB chạy Postgres + Redis + MinIO + worker. Lúc worker gọi ffmpeg trên
# video dài, RAM vọt lên và OOM killer chọn nạn nhân bừa — thường là Postgres,
# tức là chết cả hệ thống vì một job. Swap không làm nó nhanh hơn, nó chỉ giữ
# cho máy sống sót qua cơn đỉnh.
# ---------------------------------------------------------------------------
log "Swap ${SWAP_SIZE}"
if swapon --show | grep -q '/swapfile'; then
	echo "swapfile đã có, bỏ qua"
else
	fallocate -l "$SWAP_SIZE" /swapfile || dd if=/dev/zero of=/swapfile bs=1M count=4096
	chmod 600 /swapfile
	mkswap /swapfile
	swapon /swapfile
	grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >>/etc/fstab
fi
# swappiness thấp: chỉ dùng swap khi thật sự bí, không đẩy cache DB ra đĩa sớm.
cat >/etc/sysctl.d/99-voice-tool.conf <<'EOF'
vm.swappiness=10
vm.overcommit_memory=1
EOF
sysctl -p /etc/sysctl.d/99-voice-tool.conf >/dev/null

# ---------------------------------------------------------------------------
# 2. Hệ thống
# ---------------------------------------------------------------------------
log "Cập nhật hệ thống + vá bảo mật tự động"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get upgrade -y -qq
apt-get install -y -qq \
	ca-certificates curl gnupg ufw fail2ban unattended-upgrades \
	htop ncdu jq
timedatectl set-timezone "$TIMEZONE"
dpkg-reconfigure -f noninteractive unattended-upgrades

# ---------------------------------------------------------------------------
# 3. User thường
# ---------------------------------------------------------------------------
log "User ${DEPLOY_USER}"
if id "$DEPLOY_USER" &>/dev/null; then
	echo "user đã có"
else
	adduser --disabled-password --gecos "" "$DEPLOY_USER"
fi
usermod -aG sudo "$DEPLOY_USER"
# Cho phép sudo không cần mật khẩu: user này không có mật khẩu (chỉ đăng nhập
# bằng key), nên hỏi mật khẩu sudo là hỏi một thứ không tồn tại.
echo "$DEPLOY_USER ALL=(ALL) NOPASSWD:ALL" >/etc/sudoers.d/90-$DEPLOY_USER
chmod 440 /etc/sudoers.d/90-$DEPLOY_USER

if [[ -n "$SSH_PUBKEY" ]]; then
	install -d -m 700 -o "$DEPLOY_USER" -g "$DEPLOY_USER" "/home/$DEPLOY_USER/.ssh"
	echo "$SSH_PUBKEY" >"/home/$DEPLOY_USER/.ssh/authorized_keys"
	chmod 600 "/home/$DEPLOY_USER/.ssh/authorized_keys"
	chown "$DEPLOY_USER:$DEPLOY_USER" "/home/$DEPLOY_USER/.ssh/authorized_keys"
	echo "đã cài SSH key cho $DEPLOY_USER"
fi

# ---------------------------------------------------------------------------
# 4. Khoá SSH — chỉ khi đã có key, không thì tự khoá mình ngoài cửa
# ---------------------------------------------------------------------------
if [[ -n "$SSH_PUBKEY" ]]; then
	log "Khoá SSH: cấm root, tắt đăng nhập bằng mật khẩu"
	cat >/etc/ssh/sshd_config.d/99-voice-tool.conf <<'EOF'
PermitRootLogin no
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
MaxAuthTries 3
EOF
	sshd -t && systemctl reload ssh
	warn "ĐỪNG đóng phiên SSH này cho tới khi mở được phiên mới bằng key:"
	warn "    ssh -p ${SSH_PORT} ${DEPLOY_USER}@\$(hostname -I | awk '{print \$1}')"
else
	warn "Không truyền SSH key -> vẫn cho đăng nhập bằng mật khẩu và bằng root."
	warn "Đây là rủi ro lớn nhất của máy lúc này. Chạy lại script kèm key:"
	warn "    bash $0 \"\$(cat ~/.ssh/id_ed25519.pub)\""
fi

# ---------------------------------------------------------------------------
# 5. Tường lửa — nhớ cả IPv6 vì máy có IPv6 công khai
# ---------------------------------------------------------------------------
log "Tường lửa (${SSH_PORT}, 80, 443)"
sed -i 's/^IPV6=.*/IPV6=yes/' /etc/default/ufw
ufw --force reset >/dev/null
ufw default deny incoming
ufw default allow outgoing
ufw allow "${SSH_PORT}/tcp" comment 'SSH'
# Comment không được chứa dấu nháy đơn — ufw từ chối cả câu lệnh, và với
# set -e là script chết giữa chừng, để lại tường lửa cấu hình dở.
ufw allow 80/tcp comment 'HTTP'
ufw allow 443/tcp comment 'HTTPS'
ufw allow 443/udp comment 'HTTP3'
ufw --force enable

# ---------------------------------------------------------------------------
# 6. fail2ban cho sshd
# ---------------------------------------------------------------------------
log "fail2ban"
cat >/etc/fail2ban/jail.d/sshd.local <<'EOF'
[sshd]
enabled = true
maxretry = 5
findtime = 10m
bantime = 1h
EOF
systemctl enable --now fail2ban
systemctl restart fail2ban

# ---------------------------------------------------------------------------
# 7. Docker
#
# daemon.json giới hạn log ở tầng daemon: compose đã đặt cho từng service,
# nhưng đây là chốt chặn cuối cho mọi container chạy tay sau này. Không có nó,
# một container nói nhiều là ăn dần 30GB đĩa.
# ---------------------------------------------------------------------------
log "Docker"
if command -v docker &>/dev/null; then
	echo "docker đã có: $(docker --version)"
else
	install -m 0755 -d /etc/apt/keyrings
	curl -fsSL https://download.docker.com/linux/ubuntu/gpg |
		gpg --dearmor -o /etc/apt/keyrings/docker.gpg
	chmod a+r /etc/apt/keyrings/docker.gpg
	echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
		>/etc/apt/sources.list.d/docker.list
	apt-get update -qq
	apt-get install -y -qq docker-ce docker-ce-cli containerd.io \
		docker-buildx-plugin docker-compose-plugin
fi

cat >/etc/docker/daemon.json <<'EOF'
{
  "log-driver": "json-file",
  "log-opts": { "max-size": "10m", "max-file": "3" },
  "live-restore": true
}
EOF
systemctl restart docker
usermod -aG docker "$DEPLOY_USER"

install -d -o "$DEPLOY_USER" -g "$DEPLOY_USER" /opt/voice-tool
install -d -o "$DEPLOY_USER" -g "$DEPLOY_USER" /var/backups/voice-tool

# ---------------------------------------------------------------------------
log "Xong"
cat <<EOF

  Hostname : $(hostname)
  SSH      : cổng ${SSH_PORT}
  IP       : $(hostname -I)
  RAM      : $(free -h | awk '/^Mem:/ {print $2}')  |  swap: $(free -h | awk '/^Swap:/ {print $2}')
  Đĩa      : $(df -h / | awk 'NR==2 {print $4}') trống
  Docker   : $(docker --version)

  Bước tiếp theo (từ MÁY BẠN, không phải trên VPS):

    1. Đưa mã nguồn lên:
         ssh ${DEPLOY_USER}@<ip> 'git clone <repo-url> /opt/voice-tool'

    2. Tạo .env trên VPS:
         cd /opt/voice-tool && cp .env.prod.example .env && chmod 600 .env && vi .env

    3. Build và đẩy image từ máy bạn (KHÔNG build trên VPS — Next cần ~2GB RAM):
         ./scripts/push-images.sh ${DEPLOY_USER}@<ip>

    4. Chạy:
         ssh ${DEPLOY_USER}@<ip> 'cd /opt/voice-tool && ./scripts/deploy.sh'

EOF
