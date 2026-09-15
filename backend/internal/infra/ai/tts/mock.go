package tts

import (
	"context"
	"encoding/binary"
	"math"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// Mock sinh file WAV im lặng có độ dài tỉ lệ với số ký tự — dùng cho dev/test
// khi chưa có API key của nhà cung cấp thật.
type Mock struct{}

var _ domain.TTSProvider = (*Mock)(nil)

func NewMock() *Mock { return &Mock{} }

func (m *Mock) Name() string { return "mock" }

func (m *Mock) SupportedLanguages() []string { return []string{"vi", "en", "ja", "ko", "zh"} }

func (m *Mock) Synthesize(_ context.Context, req domain.SpeechRequest) ([]byte, error) {
	// ~15 ký tự/giây, tối thiểu 1 giây. Tốc độ đọc người dùng chọn đổi độ dài
	// file theo đúng tỉ lệ, để test độ dài audio ở dev không lệch với thật.
	speed := 1.0
	if req.Style.Speed != nil && *req.Style.Speed > 0 {
		speed = *req.Style.Speed
	}
	seconds := math.Max(1, float64(len([]rune(req.Text)))/15/speed)
	return silentWAV(seconds), nil
}

const (
	sampleRate = 22050
	bitDepth   = 16
	channels   = 1
)

func silentWAV(seconds float64) []byte {
	numSamples := int(seconds * sampleRate)
	dataSize := numSamples * channels * bitDepth / 8

	buf := make([]byte, 0, 44+dataSize)
	buf = append(buf, []byte("RIFF")...)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(36+dataSize))
	buf = append(buf, []byte("WAVEfmt ")...)
	buf = binary.LittleEndian.AppendUint32(buf, 16)                             // fmt chunk size
	buf = binary.LittleEndian.AppendUint16(buf, 1)                              // PCM
	buf = binary.LittleEndian.AppendUint16(buf, channels)                       //
	buf = binary.LittleEndian.AppendUint32(buf, sampleRate)                     //
	buf = binary.LittleEndian.AppendUint32(buf, sampleRate*channels*bitDepth/8) // byte rate
	buf = binary.LittleEndian.AppendUint16(buf, channels*bitDepth/8)            // block align
	buf = binary.LittleEndian.AppendUint16(buf, bitDepth)                       //
	buf = append(buf, []byte("data")...)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(dataSize))
	return append(buf, make([]byte, dataSize)...)
}
