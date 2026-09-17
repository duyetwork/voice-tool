package service

import (
	"context"
	"testing"
	"time"
)

// TestKeyGatePerKeyPace: nhịp riêng của một khoá KHÔNG được lây sang khoá khác.
//
// Đây là toàn bộ lý do SetKeyPace tồn tại: Instagram phải đi chậm (429 gần như
// mọi lượt ở nhịp chung), còn Facebook và X cùng nhịp đó thì chạy được. Hạ nhịp
// chung xuống mức của nền tảng khắt khe nhất là phạt nhầm hai nền tảng kia.
func TestKeyGatePerKeyPace(t *testing.T) {
	gate := NewKeyGate(0, 1)
	gate.SetKeyPace("instagram", 120*time.Millisecond, 1)

	ctx := context.Background()

	// Khoá KHÔNG có override: hai lượt liên tiếp gần như không phải chờ.
	release, err := gate.Acquire(ctx, "facebook")
	if err != nil {
		t.Fatalf("acquire lần 1: %v", err)
	}
	release()
	start := time.Now()
	release, err = gate.Acquire(ctx, "facebook")
	if err != nil {
		t.Fatalf("acquire lần 2: %v", err)
	}
	release()
	if waited := time.Since(start); waited > 60*time.Millisecond {
		t.Errorf("khoá không override phải đi ngay, chờ mất %v", waited)
	}

	// Khoá CÓ override: lượt thứ hai phải chờ hết khoảng nghỉ riêng.
	release, err = gate.Acquire(ctx, "instagram")
	if err != nil {
		t.Fatalf("acquire instagram lần 1: %v", err)
	}
	release()
	start = time.Now()
	release, err = gate.Acquire(ctx, "instagram")
	if err != nil {
		t.Fatalf("acquire instagram lần 2: %v", err)
	}
	release()
	if waited := time.Since(start); waited < 100*time.Millisecond {
		t.Errorf("khoá có override phải chờ ~120ms, chỉ chờ %v", waited)
	}
}

// TestKeyGateOverrideSlots: override cũng đặt lại số lượt chạy song song.
func TestKeyGateOverrideSlots(t *testing.T) {
	gate := NewKeyGate(0, 4)
	gate.SetKeyPace("instagram", 0, 1)

	ctx := context.Background()
	release, err := gate.Acquire(ctx, "instagram")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	// Slot duy nhất đang bị giữ -> lượt thứ hai phải bị chặn cho tới khi nhả.
	blocked, cancel := context.WithTimeout(ctx, 80*time.Millisecond)
	defer cancel()
	if _, err := gate.Acquire(blocked, "instagram"); err == nil {
		t.Error("slot đã bị giữ mà vẫn cấp thêm — override slots không có tác dụng")
	}
	release()

	// Nhả rồi thì lấy được ngay.
	release, err = gate.Acquire(ctx, "instagram")
	if err != nil {
		t.Fatalf("acquire sau khi nhả: %v", err)
	}
	release()
}
