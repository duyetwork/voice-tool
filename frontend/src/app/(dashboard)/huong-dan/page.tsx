"use client";

import * as React from "react";
import Link from "next/link";

import { PageHeader } from "@/components/page-header";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Table, Td, Th } from "@/components/ui/table";
import { useCollectModes, useMe } from "@/hooks/use-api";

/**
 * Hướng dẫn sử dụng — viết cho người dùng tool, không phải người phát triển.
 *
 * Đặt trong app thay vì file docs vì đây là thứ người dùng cần đọc ĐÚNG LÚC
 * đang thao tác: mỗi mục đều dẫn thẳng tới màn hình tương ứng, và trạng thái
 * hình thức thu thập (C bật hay tắt, vì sao) lấy từ chính server đang chạy chứ
 * không phải chép tay vào tài liệu rồi để nó lệch dần.
 */
export default function HuongDanPage() {
  const me = useMe();
  const modes = useCollectModes();
  const isAdmin = me.data?.role === "admin";

  const modeMeta = modes.data?.collect_modes ?? [];
  const metaOf = (mode: string) => modeMeta.find((m) => m.mode === mode);

  return (
    <>
      <PageHeader
        title="Hướng dẫn sử dụng"
        description="Tool làm gì, thao tác theo thứ tự nào, và những chỗ dễ vấp."
      />

      <div className="grid gap-6 xl:grid-cols-[1fr_320px]">
        <div className="space-y-6">
          <Section title="Mô hình 3 tầng — hiểu cái này thì mọi thứ còn lại tự sáng">
            <Flow />
            <Table>
              <thead>
                <tr>
                  <Th>Tầng</Th>
                  <Th>Là gì</Th>
                  <Th>Vì sao có nó</Th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <Td className="font-medium text-slate-900">Danh sách kênh</Td>
                  <Td>Kênh nguồn cần theo dõi, kèm điều kiện lọc</Td>
                  <Td>Không phải ngồi canh kênh rồi dán từng link</Td>
                </tr>
                <tr>
                  <Td className="font-medium text-slate-900">Bài Post</Td>
                  <Td>Một bài đăng cụ thể đã lấy được nội dung</Td>
                  <Td>
                    Duyệt trước khi tốn tiền AI, và tạo lại voice khác từ cùng bài mà không phải
                    fetch lại
                  </Td>
                </tr>
                <tr>
                  <Td className="font-medium text-slate-900">Voice</Td>
                  <Td>File audio + phần chữ đi kèm</Td>
                  <Td>Nghe thử, sửa, rồi mới đăng</Td>
                </tr>
              </tbody>
            </Table>
            <Note>
              Ngoại lệ duy nhất: <b>gõ text tay thì tạo thẳng Voice</b>, không sinh Bài Post — text
              không có bài gốc nào để truy vết về.
            </Note>
          </Section>

          <Section title="Ba hình thức tạo voice">
            <Table>
              <thead>
                <tr>
                  <Th className="w-40">Hình thức</Th>
                  <Th>Làm gì</Th>
                  <Th>Dùng khi</Th>
                  <Th className="w-28">Trạng thái</Th>
                </tr>
              </thead>
              <tbody>
                {[
                  {
                    mode: "A",
                    name: "Extract từ URL",
                    what: "Tải video/audio gốc, tách thẳng giọng nói",
                    when: "Muốn giữ nguyên giọng người trong video",
                  },
                  {
                    mode: "B",
                    name: "Text → TTS",
                    what: "Lấy nội dung bài (đúng phần chữ bạn thấy) rồi cho AI đọc",
                    when: "Bài chỉ có chữ, hoặc muốn giọng đọc thống nhất",
                  },
                  {
                    mode: "C",
                    name: "Text + Prompt → TTS",
                    what: "Lấy nội dung, cho LLM viết lại theo Prompt mẫu, rồi AI đọc bản viết lại",
                    when: "Cần tóm tắt, đổi giọng văn, chuẩn hoá bản tin",
                  },
                ].map((row) => {
                  const meta = metaOf(row.mode);
                  const enabled = meta?.enabled ?? true;
                  return (
                    <tr key={row.mode}>
                      <Td className="font-medium text-slate-900">
                        {row.mode} — {row.name}
                      </Td>
                      <Td>{row.what}</Td>
                      <Td>{row.when}</Td>
                      <Td>
                        {enabled ? (
                          <span className="text-xs font-medium text-green-700">Đang bật</span>
                        ) : (
                          <span className="text-xs text-amber-700" title={meta?.reason}>
                            Đang tắt
                          </span>
                        )}
                      </Td>
                    </tr>
                  );
                })}
              </tbody>
            </Table>
            {/* Lý do lấy thẳng từ server nên không bao giờ lệch với thực tế. */}
            {modeMeta
              .filter((m) => !m.enabled && m.reason)
              .map((m) => (
                <Note key={m.mode} tone="warn">
                  Hình thức <b>{m.mode}</b> đang tắt: {m.reason}
                </Note>
              ))}
          </Section>

          <Section title="Các bước thao tác">
            <Step n={0} title="Đăng nhập (làm 1 lần)">
              Dùng <b>chính tài khoản strongbody/multime</b> của bạn — hệ thống không có đăng ký và
              không lưu mật khẩu riêng. Điều này quan trọng ở chỗ: voice bạn tạo được đăng lên
              multime <b>dưới đúng tài khoản đó</b>, không phải một tài khoản dùng chung.
            </Step>

            <Step n={1} title="Khai API key TTS (bắt buộc nếu dùng hình thức B/C)">
              Vào <NavLink href="/ai-engines">AI Engine</NavLink> → tab <b>TTS Model</b> →{" "}
              <b>Thêm API key</b> → dán key 3voices (dạng{" "}
              <code className="rounded bg-slate-100 px-1">sk-ov-…</code>).
              <br />
              Key được mã hoá trước khi lưu và <b>không hiển thị lại</b> — bảng chỉ còn 4 ký tự cuối
              để đối chiếu. Voice của bạn chạy bằng key của bạn, nên quota và hoá đơn 3voices về
              đúng người dùng nó.
              {isAdmin ? (
                <>
                  {" "}
                  Là admin, bạn còn thấy key của mọi người và gán được 1 key cho nhiều tài khoản.
                </>
              ) : null}
            </Step>

            <Step n={2} title="Khai Bộ API key LLM (chỉ cần nếu dùng hình thức C)">
              Vào <NavLink href="/ai-engines">AI Engine</NavLink> → tab <b>LLM Model</b> →{" "}
              <b>Thêm bộ API</b>.
              <div className="mt-2 space-y-2">
                <p>
                  Một <b>bộ</b> là túi key của nhiều nhà (Gemini, OpenAI, Anthropic) chứ không phải
                  một key lẻ. Lý do: hệ thống thử lần lượt từ model rẻ nhất, hết hạn mức thì tự
                  chuyển sang nhà kế tiếp — có key của một nhà thôi thì không dự phòng được cho ai.
                </p>
                <p>
                  Cột <b>Sức khoẻ</b> của từng key nói luôn phải làm gì: <i>Đang nghỉ</i> = hết hạn
                  mức, cứ chờ tới giờ ghi trong đó; <i>Đã tắt</i> = key sai hoặc bị thu hồi, phải
                  dán key mới. Dán key mới là key tự bật lại.
                </p>
                <p>
                  Bộ dùng chung được: chọn người ở ô <b>Dùng chung với</b>. Họ dùng được bộ khi tạo
                  voice nhưng không sửa được key.
                  {isAdmin ? (
                    <>
                      {" "}
                      Là admin, bạn còn bật được <b>Hiện với mọi người</b> — nhưng nhớ rằng bật lên
                      là mở hạn mức và chi phí của bộ đó cho cả hệ thống.
                    </>
                  ) : null}
                </p>
              </div>
            </Step>

            <Step n={3} title="Tạo voice — chọn 1 trong 3 đường">
              <div className="mt-2 space-y-2">
                <p>
                  <b>Từ URL:</b> <NavLink href="/on-demand">Tạo voice</NavLink> → tab <b>Từ URL</b>{" "}
                  → dán link (YouTube, Facebook, TikTok, Instagram, X) → chọn hình thức →{" "}
                  <b>Tạo Bài Post</b>. Màn hình chuyển sang Voice, dòng mới ở trạng thái{" "}
                  <i>Đang xử lý</i> rồi thành <i>Nháp</i>.
                </p>
                <p>
                  <b>Từ text gõ tay:</b> cùng màn đó → tab <b>Nhập text</b> (hoặc nút{" "}
                  <b>+ Tạo Voice</b> ở màn <NavLink href="/voices">Voice</NavLink>). Tối đa 20.000
                  ký tự. Không sinh Bài Post.
                </p>
                <p>
                  <b>Một bước từ URL:</b> ở màn <NavLink href="/voices">Voice</NavLink> bấm{" "}
                  <b>+ Tạo Voice</b> → tab <b>Thông tin</b>: dán URL, điền sẵn tiêu đề/hashtag/ ngôn
                  ngữ/quốc gia/author/ảnh rồi bấm <b>Đăng</b>. Hộp thoại đóng ngay, hệ thống tạo
                  audio rồi tự đăng lên multime khi xong. Ô nào để trống thì lấy từ bài gốc; hashtag
                  thì gộp cả hai.
                </p>
                <p>
                  <b>Tự động theo kênh:</b>{" "}
                  <NavLink href="/lists/breaking">Danh sách Breaking</NavLink> (quét liên tục theo
                  regex) hoặc <NavLink href="/lists/scheduled">Định kỳ</NavLink> (theo tần suất
                  riêng từng kênh). Bật{" "}
                  <code className="rounded bg-slate-100 px-1">auto_process</code> thì quét xong tạo
                  voice luôn.
                </p>
              </div>
            </Step>

            <Step n={4} title="Kiểm tra và sửa">
              Ở màn <NavLink href="/voices">Voice</NavLink>: <b>▶ Nghe thử</b> / <b>⤓ Tải về</b> để
              kiểm tra audio. Nút <b>Sửa</b> có 2 tab:
              <ul className="mt-2 list-disc space-y-1 pl-5">
                <li>
                  <b>Thông tin</b> — <b>author</b> (chọn giới tính Male/Female/Other, hệ thống bốc
                  ngẫu nhiên một tài khoản Strongbody đứng tên bài đăng; bấm <b>⟳ Random</b> để bốc
                  người khác — đổi nhanh được ngay ở cột Author của bảng), tiêu đề, hashtag (bắt
                  buộc), ngôn ngữ, ảnh bìa (dán URL hoặc tải ảnh từ máy). Không đụng tới file audio.
                </li>
                <li>
                  <b>Nội dung</b> — sửa lời đọc rồi <b>Tạo lại voice</b>: đọc lại và ghi đè file cũ,
                  giữ nguyên tiêu đề/hashtag/ảnh bìa đã điền.
                </li>
              </ul>
            </Step>

            <Step n={5} title="Đăng lên multime.ai">
              Voice thiếu bất kỳ điều kiện nào dưới đây mang trạng thái <b>Chưa đủ điều kiện</b>{" "}
              (lọc được ở ô Trạng thái) và nút <b>Đăng</b> tự mờ kèm lý do: phải <b>chọn author</b>,
              có <b>tiêu đề</b>, có <b>ít nhất 1 hashtag</b> (không còn hashtag mặc định), và audio
              dài <b>tối thiểu 15 giây</b>.
              <Note>
                Đăng thành công thì file audio bị xoá khỏi hệ thống, chỉ giữ link bài trên multime —
                multime mới là nơi lưu trữ chính thức. Ảnh bìa tải từ máy cũng bị xoá theo, vì
                multime đã giữ một bản.
              </Note>
            </Step>
          </Section>

          <Section title="Tips và lưu ý">
            <Tip title="Tiêu đề là toàn bộ phần chữ của bài đăng">
              Hệ thống không có trường mô tả riêng vì multime cũng không hiển thị nó. Tiêu đề bị cắt
              còn <b>200 ký tự</b> khi đăng — nội dung dài thì sửa cho gọn trước. Hashtag được tách
              sẵn sang trường riêng, không cần xoá tay.
            </Tip>
            <Tip title="Những thứ máy tự thêm đã được bỏ">
              Số liệu tương tác của Facebook (&quot;42K views · 824 reactions&quot;), tên tài khoản
              mà X nối vào đầu tiêu đề, và tiêu đề &quot;Video by …&quot; mà công cụ tự đặt cho
              Instagram — tất cả đều bị loại, chỉ giữ chữ của người đăng.
            </Tip>
            <Tip title="Ngôn ngữ: cứ để Tự nhận diện">
              3voices đọc được{" "}
              <code className="rounded bg-slate-100 px-1">vi, en, zh, ja, ko, fr, de, es, th</code>.
              Chọn tay một tiếng ngoài danh sách thì bị từ chối kèm lý do; để <i>Tự nhận diện</i>{" "}
              thì kể cả bài tiếng Nga vẫn đọc được (3voices tự xử theo nội dung).
            </Tip>
            <Tip title="Kênh tự động: nhớ gán Bộ API và khung giờ">
              Kênh chạy hình thức C phải chọn <b>Bộ API</b> ngay trên kênh — quét tự động không có
              ai ngồi đó bấm nút để chọn, không gán thì kênh đó không tạo được voice. Phần{" "}
              <b>Lịch quét</b> cho đặt khung giờ (vd 06:00–23:00), ngày trong tuần, hoặc giờ chạy cố
              định; kênh tin tức không đăng lúc 3h sáng nên quét lúc đó chỉ tốn hạn mức.
            </Tip>

            <Tip title="TTS có giới hạn tốc độ">
              3voices cho <b>10 request/phút, 2 job đồng thời</b>. Tạo hàng loạt thì cứ để đó — gặp
              giới hạn hệ thống tự thử lại, không mất bài.
            </Tip>
            <Tip title="Voice ngắn hơn 15 giây không đăng được">
              Audio ra là WAV (~5,5MB mỗi phút). Text quá ngắn thì tạo được voice nhưng multime từ
              chối.
            </Tip>
            <Tip title="Hình thức A hay hỏng trên server hơn ở máy cá nhân">
              YouTube chặn IP máy chủ mạnh hơn IP nhà. Lỗi kiểu{" "}
              <i>&quot;Sign in to confirm you&apos;re not a bot&quot;</i> là do vậy, không phải hệ
              thống hỏng — cần cấu hình cookies cho yt-dlp.
            </Tip>
            <Tip title="Bài lỗi chạy lại được">
              Lỗi luôn hiện câu cụ thể ở cột <b>Lỗi gần nhất</b> (&quot;Tài khoản 3voices hết
              credit&quot;, &quot;Bài này bị nền tảng chặn&quot;…). Bài Post lỗi thì bấm{" "}
              <b>Chạy tạo Voice</b> lại; Voice lỗi thì <b>Sửa → Nội dung → Tạo lại voice</b>. Lỗi
              tạm thời (mạng, quá tải) hệ thống tự thử lại 3 lần.
            </Tip>
            <Tip title="Chống trùng theo ID bài đăng, không theo URL">
              Cùng một bài Facebook có nhiều dạng link vẫn chỉ tạo 1 Bài Post. Muốn tạo thêm bản nữa
              thì hệ thống hỏi lại chứ không tự quyết.
            </Tip>
          </Section>
        </div>

        <div className="space-y-6">
          <Card>
            <CardHeader title="Giới hạn cần nhớ" />
            <CardBody className="space-y-2 text-sm text-slate-700">
              <Limit label="Text gõ tay" value="20.000 ký tự" why="~25 phút audio" />
              <Limit label="Tiêu đề Voice" value="200 ký tự" why="giới hạn của multime" />
              <Limit label="Audio để đăng" value="≥ 15 giây" why="multime từ chối bài ngắn hơn" />
              <Limit label="TTS 3voices" value="10 req/phút" why="2 job đồng thời" />
              <Limit label="Regex mỗi kênh" value="tối đa 20" why="trùng lặp tự bị loại" />
            </CardBody>
          </Card>

          <Card>
            <CardHeader title="Quyền của bạn" description={me.data?.role ?? "…"} />
            <CardBody className="space-y-2 text-sm text-slate-700">
              <Perm ok={me.data?.permissions.can_write} label="Tạo, sửa, chạy, đăng voice" />
              <Perm ok={me.data?.permissions.can_delete} label="Xoá dữ liệu" />
              <Perm ok={me.data?.permissions.can_manage_users} label="Cấp quyền cho người khác" />
            </CardBody>
          </Card>
        </div>
      </div>
    </>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <Card>
      <CardHeader title={title} />
      <CardBody className="space-y-4 text-sm text-slate-700">{children}</CardBody>
    </Card>
  );
}

/** Flow vẽ đúng chuỗi 3 tầng — thứ tự này quyết định mọi thao tác phía sau. */
function Flow() {
  const steps = ["Danh sách kênh", "Bài Post", "Voice", "multime.ai"];
  return (
    <div className="flex flex-wrap items-center gap-2">
      {steps.map((s, i) => (
        <React.Fragment key={s}>
          <span className="rounded-md border border-slate-200 bg-slate-50 px-3 py-1.5 text-sm font-medium text-slate-800">
            {s}
          </span>
          {i < steps.length - 1 ? <span className="text-slate-400">→</span> : null}
        </React.Fragment>
      ))}
    </div>
  );
}

function Step({ n, title, children }: { n: number; title: string; children: React.ReactNode }) {
  return (
    <div className="flex gap-3">
      <span className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full bg-indigo-700 text-xs font-semibold text-white">
        {n}
      </span>
      <div className="min-w-0">
        <p className="font-medium text-slate-900">{title}</p>
        <div className="mt-1 text-slate-700">{children}</div>
      </div>
    </div>
  );
}

function Tip({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-md border border-slate-200 p-3">
      <p className="font-medium text-slate-900">{title}</p>
      <p className="mt-1 text-slate-600">{children}</p>
    </div>
  );
}

function Note({ children, tone = "info" }: { children: React.ReactNode; tone?: "info" | "warn" }) {
  return (
    <p
      className={
        tone === "warn"
          ? "rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-800"
          : "rounded-md bg-slate-50 px-3 py-2 text-sm text-slate-600"
      }
    >
      {children}
    </p>
  );
}

function NavLink({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <Link href={href} className="font-medium text-indigo-700 hover:underline">
      {children}
    </Link>
  );
}

function Limit({ label, value, why }: { label: string; value: string; why: string }) {
  return (
    <div className="flex items-baseline justify-between gap-2 border-b border-slate-100 pb-2 last:border-0">
      <span>{label}</span>
      <span className="text-right">
        <span className="font-medium text-slate-900">{value}</span>
        <span className="block text-xs text-slate-500">{why}</span>
      </span>
    </div>
  );
}

function Perm({ ok, label }: { ok?: boolean; label: string }) {
  return (
    <div className="flex items-center gap-2">
      <span className={ok ? "text-green-700" : "text-slate-400"}>{ok ? "✓" : "✗"}</span>
      <span className={ok ? "" : "text-slate-400"}>{label}</span>
    </div>
  );
}
