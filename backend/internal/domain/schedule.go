package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// DefaultTimezone — múi giờ mặc định của lịch quét.
//
// Lịch trước đây chạy theo giờ container (UTC). "Chỉ quét 6h–23h" mà tính theo
// UTC thì lệch 7 tiếng so với ý người dùng: kênh nghỉ đúng lúc đang đăng bài và
// quét rát đúng lúc không có gì. Khung giờ vì thế phải đi kèm múi giờ, và mặc
// định là múi giờ của người dùng chứ không của máy chủ.
const DefaultTimezone = "Asia/Ho_Chi_Minh"

// minutesPerDay — mốc trên của "phút tính từ nửa đêm".
const minutesPerDay = 24 * 60

// ChannelSchedule là khung thời gian được phép quét của 1 kênh.
//
// Zero value nghĩa là QUÉT 24/7 — đúng hành vi trước khi có tính năng này, nên
// kênh cũ không đổi cách chạy sau migration.
type ChannelSchedule struct {
	// Timezone là tên IANA (không phải offset: offset không biết tới giờ mùa hè).
	Timezone string
	// FromMin/ToMin là khung giờ hoạt động, phút tính từ nửa đêm. Phải có cả
	// hai hoặc không có cái nào. From > To = khung vắt qua nửa đêm (22:00–06:00).
	FromMin *int16
	ToMin   *int16
	// Weekdays theo quy ước time.Weekday: 0 = Chủ nhật … 6 = Thứ bảy.
	// Rỗng = mọi ngày.
	Weekdays []int16
	// FixedTimes là các mốc giờ chạy cố định (phút từ nửa đêm) — LỰA CHỌN THAY
	// THẾ cho "mỗi N phút". Chỉ Danh sách Định kỳ dùng.
	FixedTimes []int16
}

func (s ChannelSchedule) Validate() error {
	if s.Timezone != "" {
		if _, err := time.LoadLocation(s.Timezone); err != nil {
			return fmt.Errorf("%w: múi giờ %q không hợp lệ", ErrInvalidInput, s.Timezone)
		}
	}
	// Chỉ có một đầu khung giờ là cấu hình nửa vời: không có cách nào diễn giải
	// "từ 6h" mà không tự bịa ra đầu còn lại.
	if (s.FromMin == nil) != (s.ToMin == nil) {
		return fmt.Errorf(
			"%w: khung giờ quét phải có cả giờ bắt đầu và giờ kết thúc", ErrInvalidInput)
	}
	if s.FromMin != nil && *s.FromMin == *s.ToMin {
		return fmt.Errorf(
			"%w: giờ bắt đầu và giờ kết thúc trùng nhau — bỏ trống cả hai nếu muốn quét 24/7",
			ErrInvalidInput)
	}
	for _, m := range []*int16{s.FromMin, s.ToMin} {
		if m != nil && (*m < 0 || *m >= minutesPerDay) {
			return fmt.Errorf("%w: mốc giờ %d nằm ngoài khoảng 0–%d phút",
				ErrInvalidInput, *m, minutesPerDay-1)
		}
	}
	for _, d := range s.Weekdays {
		if d < 0 || d > 6 {
			return fmt.Errorf("%w: thứ trong tuần %d nằm ngoài khoảng 0–6", ErrInvalidInput, d)
		}
	}
	for _, m := range s.FixedTimes {
		if m < 0 || m >= minutesPerDay {
			return fmt.Errorf("%w: giờ chạy cố định %d nằm ngoài khoảng 0–%d phút",
				ErrInvalidInput, m, minutesPerDay-1)
		}
	}
	return nil
}

// Location trả múi giờ của kênh; cấu hình hỏng thì rơi về mặc định thay vì
// UTC — UTC là đúng cái sai mà tính năng này sinh ra để sửa.
func (s ChannelSchedule) Location() *time.Location {
	name := s.Timezone
	if name == "" {
		name = DefaultTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		if loc, err = time.LoadLocation(DefaultTimezone); err == nil {
			return loc
		}
		return time.UTC
	}
	return loc
}

// Allows cho biết thời điểm `now` có nằm trong khung được phép quét không.
func (s ChannelSchedule) Allows(now time.Time) bool {
	local := now.In(s.Location())

	if len(s.Weekdays) > 0 && !slices.Contains(s.Weekdays, int16(local.Weekday())) {
		return false
	}
	if s.FromMin == nil || s.ToMin == nil {
		return true
	}

	minute := int16(local.Hour()*60 + local.Minute())
	from, to := *s.FromMin, *s.ToMin
	if from <= to {
		return minute >= from && minute < to
	}
	// Khung vắt qua nửa đêm: 22:00–06:00 nghĩa là "từ 22h tới hết ngày, HOẶC từ
	// đầu ngày tới 6h".
	return minute >= from || minute < to
}

// NextFixedRun trả mốc chạy cố định gần nhất SAU `now`, và false nếu kênh không
// dùng giờ cố định.
//
// Có riêng hàm này thay vì để scheduler tự tính: mốc cố định phải tính trong
// MÚI GIỜ CỦA KÊNH (08:00 Hà Nội, không phải 08:00 UTC), và quy đổi đó là chỗ
// duy nhất trong hệ thống cần biết tới giờ mùa hè.
func (s ChannelSchedule) NextFixedRun(now time.Time) (time.Time, bool) {
	if len(s.FixedTimes) == 0 {
		return time.Time{}, false
	}
	times := slices.Clone(s.FixedTimes)
	slices.Sort(times)

	loc := s.Location()
	local := now.In(loc)

	// Quét tối đa 8 ngày: đủ để vượt qua mọi cấu hình weekdays (7 ngày) cộng
	// một ngày đệm cho trường hợp mốc hôm nay đã trôi qua.
	for day := range 8 {
		d := local.AddDate(0, 0, day)
		if len(s.Weekdays) > 0 && !slices.Contains(s.Weekdays, int16(d.Weekday())) {
			continue
		}
		for _, m := range times {
			candidate := time.Date(d.Year(), d.Month(), d.Day(),
				int(m)/60, int(m)%60, 0, 0, loc)
			if candidate.After(local) {
				return candidate, true
			}
		}
	}
	return time.Time{}, false
}

// Describe là câu mô tả ngắn cho log và cho màn danh sách kênh.
func (s ChannelSchedule) Describe() string {
	var parts []string
	if len(s.FixedTimes) > 0 {
		clock := make([]string, 0, len(s.FixedTimes))
		for _, m := range s.FixedTimes {
			clock = append(clock, FormatMinuteOfDay(m))
		}
		parts = append(parts, "chạy lúc "+strings.Join(clock, ", "))
	}
	if s.FromMin != nil && s.ToMin != nil {
		parts = append(parts, FormatMinuteOfDay(*s.FromMin)+"–"+FormatMinuteOfDay(*s.ToMin))
	}
	if len(s.Weekdays) > 0 {
		names := make([]string, 0, len(s.Weekdays))
		for _, d := range s.Weekdays {
			names = append(names, weekdayNames[d])
		}
		parts = append(parts, strings.Join(names, ", "))
	}
	if len(parts) == 0 {
		return "24/7"
	}
	return strings.Join(parts, " · ") + " (" + s.Location().String() + ")"
}

var weekdayNames = map[int16]string{
	0: "CN", 1: "T2", 2: "T3", 3: "T4", 4: "T5", 5: "T6", 6: "T7",
}

// FormatMinuteOfDay đổi 360 -> "06:00".
func FormatMinuteOfDay(m int16) string {
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}
