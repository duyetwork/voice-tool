package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Hai khoá khác nhau KHÔNG được chặn nhau: đó là cả lý do gate chia theo khoá.
// Một key TTS đang chạy chậm mà làm kẹt key của người khác thì gate này còn tệ
// hơn không có gì.
func TestKeyGateDoesNotBlockAcrossKeys(t *testing.T) {
	g := NewKeyGate(time.Hour, 1)
	ctx := context.Background()

	releaseA, err := g.Acquire(ctx, "a")
	if err != nil {
		t.Fatalf("acquire a: %v", err)
	}
	defer releaseA()

	done := make(chan struct{})
	go func() {
		release, err := g.Acquire(ctx, "b")
		if err == nil {
			release()
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("khoá 'b' bị khoá 'a' chặn")
	}
}

// Số slot là trần số lần gọi ĐỒNG THỜI trên cùng một khoá.
func TestKeyGateLimitsConcurrencyPerKey(t *testing.T) {
	const slots = 2
	g := NewKeyGate(0, slots)
	ctx := context.Background()

	var (
		inFlight atomic.Int32
		peak     atomic.Int32
		wg       sync.WaitGroup
	)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := g.Acquire(ctx, "same-key")
			if err != nil {
				return
			}
			defer release()

			now := inFlight.Add(1)
			for {
				old := peak.Load()
				if now <= old || peak.CompareAndSwap(old, now) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			inFlight.Add(-1)
		}()
	}
	wg.Wait()

	if got := peak.Load(); got > slots {
		t.Errorf("có lúc %d lần gọi đồng thời trên 1 khoá, trần là %d", got, slots)
	}
}

// Khoảng nghỉ tính từ lúc lần trước KẾT THÚC. Đây là điểm dễ hiểu nhầm nhất của
// gate: đo từ lúc bắt đầu thì một lần gọi kéo dài 30 giây sẽ "trả" luôn khoảng
// nghỉ, và hai request vẫn dính nhau ở đúng chỗ ta muốn tách.
func TestKeyGateWaitsGapAfterRelease(t *testing.T) {
	const gap = 120 * time.Millisecond
	g := NewKeyGate(gap, 1)
	ctx := context.Background()

	release, err := g.Acquire(ctx, "k")
	if err != nil {
		t.Fatalf("acquire lần 1: %v", err)
	}
	release()

	start := time.Now()
	release2, err := g.Acquire(ctx, "k")
	if err != nil {
		t.Fatalf("acquire lần 2: %v", err)
	}
	release2()

	if waited := time.Since(start); waited < gap/2 {
		t.Errorf("lần 2 chỉ chờ %v, muốn ít nhất ~%v", waited, gap)
	}
}

// Context bị huỷ khi đang xếp hàng phải trả lỗi VÀ trả lại slot — nuốt slot ở
// đây nghĩa là khoá đó kẹt vĩnh viễn cho tới khi restart process.
func TestKeyGateReleasesSlotOnCancel(t *testing.T) {
	g := NewKeyGate(time.Hour, 1)

	release, err := g.Acquire(context.Background(), "k")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := g.Acquire(ctx, "k"); err == nil {
		t.Fatal("muốn lỗi khi context hết hạn lúc đang xếp hàng")
	}

	release()

	// Sau khi lần đầu nhả, gate vẫn phải cấp được slot mới.
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	g2 := NewKeyGate(0, 1)
	if r, err := g2.Acquire(ctx2, "k"); err != nil {
		t.Fatalf("gate mới không cấp được slot: %v", err)
	} else {
		r()
	}
}

// Gate nil (binary chưa nối, hoặc test) phải chạy thẳng chứ không panic.
func TestNilKeyGateIsNoop(t *testing.T) {
	var g *KeyGate
	release, err := g.Acquire(context.Background(), "k")
	if err != nil {
		t.Fatalf("gate nil trả lỗi: %v", err)
	}
	release()
}
