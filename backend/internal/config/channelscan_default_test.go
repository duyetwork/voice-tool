package config

import (
	"testing"

	"github.com/spf13/viper"
)

// Ba cờ này mặc định BẬT, và điều đó chỉ đúng khi .env không nhắc tới chúng —
// đúng tình trạng của production ngày 17/09/2026 (`grep CHANNEL_SCAN .env` không
// ra dòng nào). Đặt sai mặc định thì form Thêm kênh vẫn chặn ba nền tảng mà
// không ai hiểu vì sao, vì chẳng có dòng cấu hình nào để soi.
func TestChannelScanDefaultsOn(t *testing.T) {
	v := viper.New()
	setDefaults(v)

	for _, key := range []string{
		"FACEBOOK_CHANNEL_SCAN",
		"INSTAGRAM_CHANNEL_SCAN",
		"X_CHANNEL_SCAN",
	} {
		if !v.GetBool(key) {
			t.Errorf("%s mặc định phải là true", key)
		}
	}
}

// Và vẫn phải TẮT được bằng .env — đó là toàn bộ lý do ba cờ còn tồn tại sau
// khi mặc định đổi thành bật: nền tảng nào đổi giao diện thì ngắt riêng nó.
func TestChannelScanCanBeTurnedOff(t *testing.T) {
	v := viper.New()
	setDefaults(v)
	v.Set("INSTAGRAM_CHANNEL_SCAN", false)

	if v.GetBool("INSTAGRAM_CHANNEL_SCAN") {
		t.Error("đặt false trong .env phải thắng mặc định")
	}
	if !v.GetBool("FACEBOOK_CHANNEL_SCAN") || !v.GetBool("X_CHANNEL_SCAN") {
		t.Error("ngắt một nền tảng không được làm đứt hai nền tảng còn lại")
	}
}
