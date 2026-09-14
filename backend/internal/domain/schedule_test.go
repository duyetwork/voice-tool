package domain

import (
	"errors"
	"testing"
	"time"
)

func minute(h, m int) *int16 {
	v := int16(h*60 + m)
	return &v
}

func hanoi(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(DefaultTimezone)
	if err != nil {
		t.Fatalf("không nạp được múi giờ %s: %v", DefaultTimezone, err)
	}
	return loc
}

// Zero value = 24/7: kênh tạo trước migration 000017 không được đổi cách chạy.
func TestChannelScheduleZeroValueLa247(t *testing.T) {
	var s ChannelSchedule
	for _, h := range []int{0, 3, 12, 23} {
		at := time.Date(2026, 9, 12, h, 0, 0, 0, time.UTC)
		if !s.Allows(at) {
			t.Errorf("Allows(%dh) = false, muốn true (chưa cấu hình = 24/7)", h)
		}
	}
}

// Cái sai mà tính năng này sinh ra để sửa: "chỉ quét 6h–23h" phải tính theo múi
// giờ của kênh, không phải giờ container. 22:00 UTC = 05:00 Hà Nội hôm sau, tức
// NGOÀI khung; 00:00 UTC = 07:00 Hà Nội, tức TRONG khung.
func TestChannelScheduleTheoMuiGioCuaKenh(t *testing.T) {
	s := ChannelSchedule{
		Timezone: DefaultTimezone,
		FromMin:  minute(6, 0),
		ToMin:    minute(23, 0),
	}

	cases := []struct {
		utcHour int
		want    bool
		why     string
	}{
		{22, false, "05:00 Hà Nội — ngoài khung"},
		{0, true, "07:00 Hà Nội — trong khung"},
		{16, false, "23:00 Hà Nội — đúng mốc kết thúc, đã ngoài khung"},
		{5, true, "12:00 Hà Nội — giữa khung"},
	}
	for _, tc := range cases {
		at := time.Date(2026, 9, 12, tc.utcHour, 0, 0, 0, time.UTC)
		if got := s.Allows(at); got != tc.want {
			t.Errorf("Allows(%02d:00 UTC) = %v, muốn %v (%s)", tc.utcHour, got, tc.want, tc.why)
		}
	}
}

// Khung vắt qua nửa đêm: 22:00–06:00 nghĩa là "từ 22h tới hết ngày HOẶC từ đầu
// ngày tới 6h", không phải khoảng rỗng.
func TestChannelScheduleVatQuaNuaDem(t *testing.T) {
	loc := hanoi(t)
	s := ChannelSchedule{
		Timezone: DefaultTimezone,
		FromMin:  minute(22, 0),
		ToMin:    minute(6, 0),
	}

	cases := []struct {
		hour int
		want bool
	}{
		{23, true},
		{2, true},
		{5, true},
		{6, false},
		{12, false},
		{21, false},
	}
	for _, tc := range cases {
		at := time.Date(2026, 9, 12, tc.hour, 0, 0, 0, loc)
		if got := s.Allows(at); got != tc.want {
			t.Errorf("Allows(%02d:00 Hà Nội) = %v, muốn %v", tc.hour, got, tc.want)
		}
	}
}

// 2026-09-12 là thứ Bảy (Weekday = 6).
func TestChannelScheduleTheoThu(t *testing.T) {
	loc := hanoi(t)
	saturday := time.Date(2026, 9, 12, 10, 0, 0, 0, loc)
	if saturday.Weekday() != time.Saturday {
		t.Fatalf("mốc thử không phải thứ Bảy: %v", saturday.Weekday())
	}

	workdays := ChannelSchedule{Timezone: DefaultTimezone, Weekdays: []int16{1, 2, 3, 4, 5}}
	if workdays.Allows(saturday) {
		t.Error("kênh chỉ chạy ngày làm việc mà vẫn quét thứ Bảy")
	}
	if !workdays.Allows(saturday.AddDate(0, 0, 2)) {
		t.Error("thứ Hai phải quét được")
	}
}

func TestChannelScheduleNextFixedRun(t *testing.T) {
	loc := hanoi(t)
	s := ChannelSchedule{
		Timezone:   DefaultTimezone,
		FixedTimes: []int16{18 * 60, 8 * 60, 12 * 60}, // cố tình để lệch thứ tự
	}

	// 09:00 -> mốc kế tiếp là 12:00 cùng ngày.
	next, ok := s.NextFixedRun(time.Date(2026, 9, 12, 9, 0, 0, 0, loc))
	if !ok {
		t.Fatal("NextFixedRun() = false, muốn true")
	}
	if next.Hour() != 12 || next.Day() != 12 {
		t.Errorf("mốc kế tiếp = %v, muốn 12:00 ngày 12", next)
	}

	// 19:00 -> đã qua hết mốc trong ngày, nhảy sang 08:00 hôm sau.
	next, _ = s.NextFixedRun(time.Date(2026, 9, 12, 19, 0, 0, 0, loc))
	if next.Hour() != 8 || next.Day() != 13 {
		t.Errorf("mốc kế tiếp = %v, muốn 08:00 ngày 13", next)
	}

	// Không cấu hình giờ cố định thì kênh chạy theo "mỗi N phút".
	if _, ok := (ChannelSchedule{}).NextFixedRun(time.Now()); ok {
		t.Error("không có fixed_times mà vẫn trả mốc chạy")
	}
}

func TestChannelScheduleValidate(t *testing.T) {
	bad := []struct {
		name string
		s    ChannelSchedule
	}{
		{"chỉ có giờ bắt đầu", ChannelSchedule{FromMin: minute(6, 0)}},
		{"chỉ có giờ kết thúc", ChannelSchedule{ToMin: minute(23, 0)}},
		{"hai đầu trùng nhau", ChannelSchedule{FromMin: minute(6, 0), ToMin: minute(6, 0)}},
		{"múi giờ không có thật", ChannelSchedule{Timezone: "Asia/Atlantis"}},
		{"thứ ngoài 0–6", ChannelSchedule{Weekdays: []int16{7}}},
		{"giờ cố định ngoài ngày", ChannelSchedule{FixedTimes: []int16{2000}}},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.s.Validate(); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("Validate() = %v, muốn ErrInvalidInput", err)
			}
		})
	}

	ok := ChannelSchedule{
		Timezone: DefaultTimezone,
		FromMin:  minute(6, 0),
		ToMin:    minute(23, 0),
		Weekdays: []int16{1, 2, 3, 4, 5},
	}
	if err := ok.Validate(); err != nil {
		t.Errorf("cấu hình hợp lệ mà báo lỗi: %v", err)
	}
}

// Múi giờ hỏng phải rơi về mặc định, KHÔNG rơi về UTC — UTC chính là cái sai
// tính năng này sinh ra để sửa.
func TestChannelScheduleLocationKhongRoiVeUTC(t *testing.T) {
	s := ChannelSchedule{Timezone: "Asia/Atlantis"}
	if got := s.Location().String(); got != DefaultTimezone {
		t.Errorf("Location() = %s, muốn %s", got, DefaultTimezone)
	}
}
