/**
 * Danh sách ngôn ngữ dùng cho mọi ô chọn "Ngôn ngữ" (Bài Post, Voice, Danh sách).
 *
 * `auto` là mặc định của hệ thống — để nền tảng nguồn / multime.ai tự nhận
 * diện, vì đoán sai ngôn ngữ tệ hơn là không đoán.
 *
 * Mã ngôn ngữ theo ISO 639-1; hai biến thể cần phân biệt vùng thì dùng dạng
 * đầy đủ (`zh-CN`, `zh-TW`, `pt-BR`). Backend so khớp với
 * `supported_languages` của AI Engine bằng phần gốc trước dấu `-`, nên
 * `pt-BR` vẫn khớp engine khai báo `pt`.
 *
 * Thứ tự giữ đúng thứ tự nghiệp vụ đưa xuống, không sắp lại theo alphabet.
 */
export interface LanguageOption {
  value: string;
  label: string;
}

export const LANGUAGE_AUTO = "auto";

export const LANGUAGE_OPTIONS: LanguageOption[] = [
  { value: LANGUAGE_AUTO, label: "Tự nhận diện" },
  { value: "en", label: "English (en)" },
  { value: "vi", label: "Vietnamese (vi)" },
  { value: "zh-CN", label: "Chinese (Simplified) (zh-CN)" },
  { value: "zh-TW", label: "Chinese (Traditional) (zh-TW)" },
  { value: "hi", label: "Hindi (hi)" },
  { value: "es", label: "Spanish (es)" },
  { value: "pt-BR", label: "Portuguese (Brazil) (pt-BR)" },
  { value: "ru", label: "Russian (ru)" },
  { value: "fr", label: "French (fr)" },
  { value: "de", label: "German (de)" },
  { value: "ja", label: "Japanese (ja)" },
  { value: "ko", label: "Korean (ko)" },
  { value: "tr", label: "Turkish (tr)" },
  { value: "id", label: "Indonesian (id)" },
  { value: "it", label: "Italian (it)" },
  { value: "ar", label: "Arabic (ar)" },
  { value: "uk", label: "Ukrainian (uk)" },
  { value: "bn", label: "Bengali (bn)" },
  { value: "lv", label: "Latvian (lv)" },
  { value: "lt", label: "Lithuanian (lt)" },
  { value: "hy", label: "Armenian (hy)" },
  { value: "ka", label: "Georgian (ka)" },
  { value: "mn", label: "Mongolian (mn)" },
  { value: "ky", label: "Kyrgyz (ky)" },
  { value: "tg", label: "Tajik (tg)" },
  { value: "tk", label: "Turkmen (tk)" },
  { value: "lo", label: "Lao (lo)" },
  { value: "sw", label: "Swahili (sw)" },
  { value: "am", label: "Amharic (am)" },
  { value: "ha", label: "Hausa (ha)" },
  { value: "zu", label: "Zulu (zu)" },
  { value: "af", label: "Afrikaans (af)" },
  { value: "so", label: "Somali (so)" },
  { value: "ps", label: "Pashto (ps)" },
  { value: "is", label: "Icelandic (is)" },
  { value: "ga", label: "Irish (ga)" },
  { value: "cy", label: "Welsh (cy)" },
  { value: "ca", label: "Catalan (ca)" },
  { value: "mt", label: "Maltese (mt)" },
  { value: "pt", label: "Portuguese (pt)" },
  { value: "pa", label: "Punjabi (pa)" },
  { value: "te", label: "Telugu (te)" },
  { value: "ur", label: "Urdu (ur)" },
  { value: "ta", label: "Tamil (ta)" },
  { value: "mr", label: "Marathi (mr)" },
  { value: "th", label: "Thai (th)" },
  { value: "gu", label: "Gujarati (gu)" },
  { value: "pl", label: "Polish (pl)" },
  { value: "kn", label: "Kannada (kn)" },
  { value: "ml", label: "Malayalam (ml)" },
  { value: "nl", label: "Dutch (nl)" },
  { value: "ms", label: "Malay (ms)" },
  { value: "fa", label: "Persian (fa)" },
  { value: "ro", label: "Romanian (ro)" },
  { value: "ne", label: "Nepali (ne)" },
  { value: "si", label: "Sinhala (si)" },
  { value: "cs", label: "Czech (cs)" },
  { value: "hu", label: "Hungarian (hu)" },
  { value: "az", label: "Azerbaijani (az)" },
  { value: "kk", label: "Kazakh (kk)" },
  { value: "he", label: "Hebrew (he)" },
  { value: "el", label: "Greek (el)" },
  { value: "bg", label: "Bulgarian (bg)" },
  { value: "uz", label: "Uzbek (uz)" },
  { value: "fi", label: "Finnish (fi)" },
  { value: "sv", label: "Swedish (sv)" },
  { value: "no", label: "Norwegian (no)" },
  { value: "da", label: "Danish (da)" },
  { value: "sk", label: "Slovak (sk)" },
  { value: "hr", label: "Croatian (hr)" },
  { value: "sr", label: "Serbian (sr)" },
  { value: "sl", label: "Slovenian (sl)" },
  { value: "mk", label: "Macedonian (mk)" },
  { value: "sq", label: "Albanian (sq)" },
  { value: "et", label: "Estonian (et)" },
];

export function languageLabel(value?: string | null): string {
  if (!value) return "—";
  return LANGUAGE_OPTIONS.find((o) => o.value === value)?.label ?? value;
}

/**
 * languageOptionsFor: danh sách cho 1 ô chọn cụ thể. Giá trị đang lưu không nằm trong
 * danh sách (dữ liệu cũ, hoặc nền tảng trả mã lạ) thì thêm nó vào đầu để không
 * âm thầm đổi ngôn ngữ của bản ghi khi người dùng sửa field khác.
 */
export function languageOptionsFor(current?: string | null): LanguageOption[] {
  if (!current || LANGUAGE_OPTIONS.some((o) => o.value === current)) return LANGUAGE_OPTIONS;
  return [{ value: current, label: current }, ...LANGUAGE_OPTIONS];
}

/**
 * compactLabel bỏ phần mã trong ngoặc cuối nhãn: "Vietnamese (vi)" ->
 * "Vietnamese". Dùng cho ô chọn nằm TRONG bảng, nơi cột cần hẹp; ô lọc và form
 * vẫn dùng nhãn đầy đủ có mã để chọn không nhầm.
 */
export function compactLabel(label: string): string {
  return label.replace(/\s*\([^()]*\)$/, "");
}

/** Danh sách nhãn ngắn cho 1 ô chọn trong bảng. */
export function compactLanguageOptions(current?: string | null): LanguageOption[] {
  return languageOptionsFor(current).map((o) => ({ ...o, label: compactLabel(o.label) }));
}
