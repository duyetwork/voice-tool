"use client";

import * as React from "react";

import { Button } from "@/components/ui/button";
import { Field, Select } from "@/components/ui/field";
import { useVoiceStyleOptions } from "@/hooks/use-api";
import type { Gender, VoiceStyle } from "@/types/api";

/**
 * Mục "Cấu hình giọng đọc" của form tạo/sửa Voice.
 *
 * Đóng lại theo mặc định vì phần lớn bản tin đọc bằng giọng mặc định là xong;
 * nhưng cái ĐẦU MỤC thì luôn hiện, ngay trong modal, trước khi bấm Đăng — một
 * chỗ chỉnh giọng mà phải sang màn khác mới thấy thì cũng như không có.
 *
 * Danh sách giá trị lấy từ `/meta/voice-style` chứ không chép cứng: đây là bộ
 * từ khoá của nhà cung cấp TTS và backend là chỗ duy nhất validate nó. Chép
 * sang đây là tự tạo một bản sao sớm muộn cũng lệch, và bên lệch sẽ là bên
 * không validate.
 */

/** Nhãn tiếng Việt cho bộ từ khoá của nhà cung cấp. */
const GENDER_LABELS: Record<string, string> = {
  female: "Nữ",
  male: "Nam",
};

const AGE_LABELS: Record<string, string> = {
  child: "Trẻ em",
  teenager: "Thiếu niên",
  "young adult": "Thanh niên",
  "middle-aged": "Trung niên",
  elderly: "Lớn tuổi",
};

const PITCH_LABELS: Record<string, string> = {
  "very low pitch": "Rất trầm",
  "low pitch": "Trầm",
  "moderate pitch": "Vừa",
  "high pitch": "Cao",
  "very high pitch": "Rất cao",
  whisper: "Thì thầm",
};

const ACCENT_LABELS: Record<string, string> = {
  "american accent": "Giọng Mỹ",
  "australian accent": "Giọng Úc",
  "british accent": "Giọng Anh",
  "canadian accent": "Giọng Canada",
  "chinese accent": "Giọng Trung",
  "indian accent": "Giọng Ấn",
  "japanese accent": "Giọng Nhật",
  "korean accent": "Giọng Hàn",
  "portuguese accent": "Giọng Bồ Đào Nha",
  "russian accent": "Giọng Nga",
};

/**
 * labelOf rơi về NGUYÊN VĂN giá trị khi chưa có nhãn tiếng Việt.
 *
 * Nhà cung cấp thêm từ khoá mới thì nó hiện ra ngay và vẫn chọn được, thay vì
 * thành một dòng trống trong danh sách chỉ vì frontend chưa kịp dịch.
 */
function labelOf(map: Record<string, string>, value: string): string {
  return map[value] ?? value;
}

/** Cấu hình rỗng = "để mặc định của nhà cung cấp". */
export function isEmptyVoiceStyle(style: VoiceStyle): boolean {
  return !style.gender && !style.age && !style.pitch && !style.accent && style.speed === undefined;
}

/** Giới tính giọng suy từ metadata voice; "other" thì không suy được. */
function genderFromAuthor(authorGender: Gender | null | undefined): string {
  return authorGender === "male" || authorGender === "female" ? authorGender : "";
}

export function VoiceStyleFields({
  value,
  onChange,
  authorGender,
}: {
  value: VoiceStyle;
  onChange: (next: VoiceStyle) => void;
  /**
   * Giới tính tài khoản đứng tên bài đăng. Điền sẵn giới tính giọng theo nó:
   * bài của tài khoản nam mà đọc bằng giọng nữ là thứ gần như không ai muốn,
   * và bắt chọn lại một thứ vừa khai ở ô ngay trên là một lần bấm thừa.
   */
  authorGender?: Gender | null;
}) {
  const options = useVoiceStyleOptions();

  // Mở sẵn khi voice ĐÃ có cấu hình (màn Tạo lại): mở form mà không thấy giọng
  // đang chạy thì người dùng tưởng nó đang chạy giọng mặc định.
  const [open, setOpen] = React.useState(() => !isEmptyVoiceStyle(value));

  // Điền sẵn chỉ áp dụng khi người dùng CHƯA tự chọn giới tính giọng. Voice đã
  // có cấu hình sẵn cũng tính là đã chọn — đổi tài khoản đứng tên không được
  // âm thầm thay giọng họ đã chốt.
  const genderPicked = React.useRef(Boolean(value.gender));
  const derivedGender = genderFromAuthor(authorGender);

  React.useEffect(() => {
    if (genderPicked.current || !derivedGender) return;
    if (value.gender === derivedGender) return;
    onChange({ ...value, gender: derivedGender });
    // value/onChange đổi theo từng thao tác ở các ô khác; chỉ chạy lại khi
    // giới tính suy ra từ metadata thực sự đổi.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [derivedGender]);

  const set = (patch: Partial<VoiceStyle>) => onChange({ ...value, ...patch });

  const setChoice = (key: "gender" | "age" | "pitch" | "accent", next: string) => {
    if (key === "gender") genderPicked.current = next !== "";
    // Chuỗi rỗng = "theo mặc định" -> xoá hẳn khỏi object thay vì gửi "" lên:
    // backend coi trường trống là chưa chọn, còn "" chỉ là một trường rỗng nằm
    // lại trong JSONB.
    const patch: VoiceStyle = { ...value };
    if (next) patch[key] = next;
    else delete patch[key];
    onChange(patch);
  };

  const speedMin = options.data?.speed.min ?? 0.5;
  const speedMax = options.data?.speed.max ?? 2.0;
  const speed = value.speed ?? 1.0;

  const empty = isEmptyVoiceStyle(value);
  // Giới tính tài khoản là "Khác" thì không suy ra được nam hay nữ — nói thẳng
  // là phải tự chọn, thay vì để ô trống không rõ vì sao.
  const needsGenderChoice = authorGender === "other" && !value.gender;

  return (
    <div className="rounded-md border border-slate-300">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className="flex w-full items-center justify-between gap-2 px-3 py-2.5 text-left hover:bg-slate-50"
      >
        <span className="text-sm font-medium text-slate-700">
          Cấu hình giọng đọc
          <span className="ml-2 text-xs font-normal text-slate-500">
            {empty ? "Đang dùng giọng mặc định" : summarize(value)}
          </span>
        </span>
        <span aria-hidden="true" className="shrink-0 text-xs text-slate-500">
          {open ? "Thu gọn ▲" : "Mở rộng ▼"}
        </span>
      </button>

      {open ? (
        <div className="space-y-3 border-t border-slate-200 px-3 py-3">
          {options.isPending ? (
            <p className="text-sm text-slate-500">Đang tải danh sách giọng…</p>
          ) : options.isError ? (
            <p className="text-sm text-red-700">
              Không tải được danh sách giọng hợp lệ — bỏ qua mục này thì voice vẫn chạy bằng giọng
              mặc định.
            </p>
          ) : (
            <>
              {/* Người này đã khai sẵn một giọng ở AI Engine: chỉnh ở đây là BỎ
                  giọng đó. Nói trước, vì nếu không thì họ chỉnh xong nghe ra
                  một giọng lạ mà không có chỗ nào giải thích. */}
              {options.data?.has_saved_voice && !empty ? (
                <p className="rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-800">
                  Có cấu hình ở đây thì hệ thống bỏ giọng đã lưu của bạn và sinh giọng mới theo mô
                  tả.
                </p>
              ) : null}

              <div className="grid gap-3 sm:grid-cols-2">
                <Field
                  label="Giới tính giọng"
                  hint={needsGenderChoice ? "Tài khoản để “Khác” — chọn Nam hoặc Nữ." : undefined}
                >
                  <Select
                    value={value.gender ?? ""}
                    onChange={(e) => setChoice("gender", e.target.value)}
                  >
                    <option value="">— Mặc định —</option>
                    {(options.data?.genders ?? []).map((g) => (
                      <option key={g} value={g}>
                        {labelOf(GENDER_LABELS, g)}
                      </option>
                    ))}
                  </Select>
                </Field>

                <Field label="Độ tuổi giọng">
                  <Select
                    value={value.age ?? ""}
                    onChange={(e) => setChoice("age", e.target.value)}
                  >
                    <option value="">— Mặc định —</option>
                    {(options.data?.ages ?? []).map((a) => (
                      <option key={a} value={a}>
                        {labelOf(AGE_LABELS, a)}
                      </option>
                    ))}
                  </Select>
                </Field>

                <Field label="Cao độ">
                  <Select
                    value={value.pitch ?? ""}
                    onChange={(e) => setChoice("pitch", e.target.value)}
                  >
                    <option value="">— Mặc định —</option>
                    {(options.data?.pitches ?? []).map((p) => (
                      <option key={p} value={p}>
                        {labelOf(PITCH_LABELS, p)}
                      </option>
                    ))}
                  </Select>
                </Field>

                <Field label="Giọng vùng miền">
                  <Select
                    value={value.accent ?? ""}
                    onChange={(e) => setChoice("accent", e.target.value)}
                  >
                    <option value="">— Không —</option>
                    {(options.data?.accents ?? []).map((a) => (
                      <option key={a} value={a}>
                        {labelOf(ACCENT_LABELS, a)}
                      </option>
                    ))}
                  </Select>
                </Field>
              </div>

              <Field label={`Tốc độ đọc: ${speed.toFixed(2)}×`}>
                <div className="flex items-center gap-3">
                  <span className="text-xs text-slate-500">{speedMin.toFixed(1)}×</span>
                  <input
                    type="range"
                    min={speedMin}
                    max={speedMax}
                    step={0.05}
                    value={speed}
                    onChange={(e) => set({ speed: Number(e.target.value) })}
                    className="h-1.5 w-full accent-indigo-700"
                  />
                  <span className="text-xs text-slate-500">{speedMax.toFixed(1)}×</span>
                </div>
              </Field>

              {/* Một nút về mặc định, vì đưa 5 ô về trống bằng tay là 5 lần bấm
                  và rất dễ sót một ô rồi tưởng đã bỏ hết cấu hình. */}
              <div className="flex justify-end">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={empty}
                  onClick={() => {
                    genderPicked.current = false;
                    onChange({});
                  }}
                >
                  Về mặc định
                </Button>
              </div>
            </>
          )}
        </div>
      ) : null}
    </div>
  );
}

/** Tóm tắt cấu hình cho dòng đầu mục — đọc được mà không cần mở ra. */
function summarize(style: VoiceStyle): string {
  return [
    style.gender ? labelOf(GENDER_LABELS, style.gender) : "",
    style.age ? labelOf(AGE_LABELS, style.age) : "",
    style.pitch ? labelOf(PITCH_LABELS, style.pitch) : "",
    style.accent ? labelOf(ACCENT_LABELS, style.accent) : "",
    style.speed !== undefined && style.speed !== 1 ? `${style.speed.toFixed(2)}×` : "",
  ]
    .filter(Boolean)
    .join(" · ");
}
