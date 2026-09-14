"use client";

import * as React from "react";
import Link from "next/link";

import { PageHeader } from "@/components/page-header";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Table, Td, Th } from "@/components/ui/table";
import { useCollectModes, useMe, usePlatforms } from "@/hooks/use-api";

/**
 * Hướng dẫn sử dụng — viết cho người dùng tool, không phải người phát triển.
 *
 * Tổ chức theo MÀN HÌNH chứ không theo khái niệm: người mở trang này gần như
 * luôn đang đứng ở một màn cụ thể và vướng một thao tác cụ thể, nên thứ họ cần
 * là "màn Danh sách kênh Định kỳ, phần thêm kênh, ô này nghĩa là gì" — không
 * phải một bài giảng về kiến trúc. Mỗi màn có đủ: các bước thao tác, giải thích
 * từng ô trong hộp thoại, và ý nghĩa từng cột trong bảng.
 *
 * Đặt trong app thay vì file docs vì đây là thứ phải đọc ĐÚNG LÚC đang thao
 * tác: mỗi mục dẫn thẳng tới màn hình tương ứng, và những thứ phụ thuộc cấu
 * hình server (hình thức C bật hay tắt, nền tảng nào quét được cả kênh) lấy từ
 * chính server đang chạy chứ không chép tay rồi để nó lệch dần.
 */

/** TOC là nguồn duy nhất của mục lục VÀ của id các mục — hai thứ không được lệch. */
const TOC: { id: string; label: string; sub?: { id: string; label: string }[] }[] = [
  { id: "tong-quan", label: "1. Hiểu mô hình 3 tầng" },
  {
    id: "kenh-dinh-ky",
    label: "2. Danh sách kênh Định kỳ",
    sub: [
      { id: "dk-them", label: "2.1 Thêm kênh" },
      { id: "dk-thamso", label: "2.2 Tham số quét" },
      { id: "dk-lich", label: "2.3 Lịch quét" },
      { id: "dk-logic", label: "2.4 Kênh quét ra bài như thế nào" },
      { id: "dk-bang", label: "2.5 Các cột trong bảng" },
      { id: "dk-sua", label: "2.6 Sửa / bật tắt / xoá kênh" },
    ],
  },
  {
    id: "kenh-breaking",
    label: "3. Danh sách kênh Breaking",
    sub: [
      { id: "bk-khacgi", label: "3.1 Khác Định kỳ ở đâu" },
      { id: "bk-regex", label: "3.2 Điều kiện bắt bài" },
    ],
  },
  {
    id: "tao-voice",
    label: "4. Tạo Voice thủ công",
    sub: [
      { id: "tv-hinhthuc", label: "4.1 Ba hình thức tạo" },
      { id: "tv-form", label: "4.2 Từng ô trong hộp thoại" },
    ],
  },
  {
    id: "man-voice",
    label: "5. Màn Voice",
    sub: [
      { id: "v-bang", label: "5.1 Các cột và trạng thái" },
      { id: "v-thongtin", label: "5.2 Sửa tab Thông tin" },
      { id: "v-noidung", label: "5.3 Sửa tab Nội dung" },
      { id: "v-dang", label: "5.4 Đăng lên multime" },
    ],
  },
  { id: "man-baipost", label: "6. Màn Bài Post" },
  {
    id: "danh-muc",
    label: "7. Prompt mẫu & AI Engine",
    sub: [
      { id: "dm-prompt", label: "7.1 Prompt mẫu" },
      { id: "dm-tts", label: "7.2 API key TTS" },
      { id: "dm-llm", label: "7.3 Bộ API key LLM" },
    ],
  },
  { id: "su-co", label: "8. Gặp sự cố thì xem ở đâu" },
];

export default function HuongDanPage() {
  const me = useMe();
  const modes = useCollectModes();
  const platforms = usePlatforms();
  const isAdmin = me.data?.role === "admin";

  const modeMeta = modes.data?.collect_modes ?? [];
  const metaOf = (mode: string) => modeMeta.find((m) => m.mode === mode);
  const channelScan = platforms.data?.channel_scan ?? [];

  return (
    <>
      <PageHeader
        title="Hướng dẫn sử dụng"
        description="Từng màn hình, từng thao tác, từng ô trong hộp thoại."
      />

      <div className="grid gap-6 xl:grid-cols-[1fr_320px]">
        <div className="space-y-6">
          {/* ------------------------------------------------------------ */}
          <Section id="tong-quan" title="1. Hiểu mô hình 3 tầng">
            <p>
              Mọi thứ trong tool chạy theo đúng một chuỗi. Nắm chuỗi này thì mọi màn hình còn lại
              tự sáng:
            </p>
            <Flow />
            <ul className="ml-4 list-disc space-y-1">
              <li>
                <b>Danh sách kênh</b> — nơi khai kênh nguồn. Hệ thống tự quét theo lịch và đẻ ra
                Bài Post. Không bắt buộc: tạo voice tay thì bỏ qua tầng này.
              </li>
              <li>
                <b>Bài Post</b> — một bài đăng đã lấy về, kèm nội dung và metadata gốc. Đây là nơi
                duyệt trước khi tốn tiền AI.
              </li>
              <li>
                <b>Voice</b> — file audio + phần chữ đi kèm (tiêu đề, hashtag, ảnh bìa, tác giả).
              </li>
              <li>
                <b>multime.ai</b> — đích đến. Voice chỉ đăng được khi đã đủ tiêu đề, hashtag, tác
                giả và dài hơn mức tối thiểu.
              </li>
            </ul>
            <Note>
              Voice gõ tay là ngoại lệ duy nhất: nó không sinh Bài Post, vì không có bài gốc nào để
              truy vết lại.
            </Note>
          </Section>

          {/* ------------------------------------------------------------ */}
          <Section id="kenh-dinh-ky" title="2. Danh sách kênh Định kỳ">
            <p>
              <NavLink href="/lists/scheduled">Mở màn Định kỳ</NavLink> — kênh quét theo{" "}
              <b>tần suất</b> của riêng nó và lấy <b>mọi bài mới</b> kể từ lần quét trước. Dùng cho
              kênh mà bài nào cũng cần.
            </p>

            <SubSection id="dk-them" title="2.1 Thêm kênh">
              <Step n={1} title="Bấm + Thêm kênh">
                Hộp thoại mở ra là hộp thoại dùng chung cho cả thêm và sửa — mọi thứ khai ở đây đều
                sửa lại được sau.
              </Step>
              <Step n={2} title="Dán URL kênh nguồn">
                URL của <i>cả kênh</i>, không phải một bài. Ví dụ{" "}
                <code className="rounded bg-slate-100 px-1">
                  https://www.youtube.com/@kenh/videos
                </code>
                . Nền tảng được nhận ra từ chính URL, không có ô chọn riêng.
              </Step>
              <Step n={3} title="Chọn hình thức tạo">
                Xem <a className="text-indigo-700 hover:underline" href="#tv-hinhthuc">mục 4.1</a>.
                Hình thức C bắt buộc chọn thêm <b>Prompt mẫu</b> và <b>Bộ API</b> — quét tự động
                không có ai bấm nút để chọn, nên hai thứ đó phải nằm sẵn trên kênh.
              </Step>
              <Step n={4} title="Chọn tần suất quét và ngôn ngữ">
                Tần suất tối thiểu 5 phút. Ngôn ngữ để <i>Tự nhận diện</i> nếu kênh đa ngữ; chọn
                cụ thể thì mọi Bài Post và Voice của kênh nhận ngôn ngữ đó.
              </Step>
              <Step n={5} title="Chọn hai ô tự động (nếu muốn)">
                Xem <a className="text-indigo-700 hover:underline" href="#dk-tudong">ngay bên dưới</a>.
              </Step>
              <Step n={6} title="Bấm Thêm kênh">
                Kênh chạy vòng quét đầu tiên gần như ngay sau đó.
              </Step>

              <Tip id="dk-tudong" title="Hai ô tự động — mặc định TẮT cả hai">
                <b>Tự động tạo Voice và đăng lên multime.ai</b>: quét được bài là tạo voice và đăng
                luôn, không cần ai bấm gì. Tắt thì bài chỉ nằm ở màn Bài Post chờ duyệt — dùng khi
                muốn gom lại duyệt hàng loạt cho đỡ tốn chi phí AI.
                <br />
                <b>Random author</b>: bốc một tài khoản đứng tên bài đăng, lọc theo quốc gia suy ra
                từ ngôn ngữ của kênh. Bật ô thứ nhất thì ô này <b>tự bật theo</b>, vì multime bắt
                buộc bài phải có tác giả — không có thì voice chạy xong rồi hỏng ở đúng bước cuối.
                Vẫn bỏ tick lại được nếu bạn muốn tự gán tác giả bằng tay sau.
              </Tip>

              <Tip title="Chỉ YouTube và TikTok quét được CẢ KÊNH">
                Giới hạn của yt-dlp, không phải cấu hình sai. Trạng thái thật của server đang chạy:
                <div className="mt-2 space-y-1">
                  {channelScan.map((p) => (
                    <div key={p.platform} className="flex gap-2">
                      <span className={p.enabled ? "text-green-700" : "text-amber-700"}>
                        {p.enabled ? "✓" : "✗"}
                      </span>
                      <span>
                        <b>{p.platform}</b>
                        {p.reason ? <span className="text-slate-500"> — {p.reason}</span> : null}
                      </span>
                    </div>
                  ))}
                </div>
                <span className="mt-2 block">
                  Thêm kênh trên nền tảng không quét được sẽ bị từ chối ngay ở form. Lấy bài{" "}
                  <b>lẻ từ URL</b> thì cả năm nền tảng đều chạy bình thường.
                </span>
              </Tip>
            </SubSection>

            <SubSection id="dk-thamso" title="2.2 Tham số quét (nâng cao)">
              <p>
                Ba con số dễ nhầm nhất trong tool. Cả ba đều <b>điền sẵn giá trị mặc định</b>; xoá
                trắng ô nào thì ô đó về 0.
              </p>
              <Table>
                <thead>
                  <tr>
                    <Th className="w-56">Ô</Th>
                    <Th>Trả lời câu hỏi</Th>
                    <Th className="w-40">Mặc định</Th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <Td className="font-medium text-slate-900">Số bài mỗi lần quét</Td>
                    <Td>
                      Mỗi vòng quét nhìn bao nhiêu bài mới nhất của kênh để dò bài mới? Đây là{" "}
                      <i>cửa sổ</i> — kênh đăng nhiều hơn số này giữa hai lần quét thì phần dôi ra
                      bị bỏ sót.
                    </Td>
                    <Td>50</Td>
                  </tr>
                  <tr>
                    <Td className="font-medium text-slate-900">Số bài cũ của kênh</Td>
                    <Td>
                      Lúc <i>thêm</i> kênh, lấy về bao nhiêu bài đã đăng từ trước? Chỉ có tác dụng
                      đúng một lần, ở vòng quét đầu tiên. 0 = chỉ lấy bài đăng sau khi thêm kênh.
                    </Td>
                    <Td>50</Td>
                  </tr>
                  <tr>
                    <Td className="font-medium text-slate-900">Trần Bài Post mỗi vòng</Td>
                    <Td>
                      Mỗi vòng được <i>tạo</i> tối đa bao nhiêu Bài Post? Chặn nổ chi phí AI khi
                      kênh đăng ồ ạt. Phần vượt <b>không mất</b> — nó nằm lại cho vòng sau. 0 =
                      không giới hạn.
                    </Td>
                    <Td>50</Td>
                  </tr>
                </tbody>
              </Table>
              <Note>
                Đặt <b>Trần Bài Post</b> lớn hơn <b>Số bài mỗi lần quét</b> thì phần vượt không có
                tác dụng — một vòng không thể tạo ra nhiều bài hơn số bài nó nhìn thấy. Form cảnh
                báo đỏ nhưng vẫn cho lưu, vì đó là cách hợp lệ để nói &quot;coi như không giới
                hạn&quot;.
              </Note>
            </SubSection>

            <SubSection id="dk-lich" title="2.3 Lịch quét">
              <p>
                Mở khối <b>Lịch quét</b> trong hộp thoại. Mặc định đã điền sẵn{" "}
                <b>00:00–23:59, cả 7 ngày</b> — tức là quét cả ngày mọi ngày.
              </p>
              <ul className="ml-4 list-disc space-y-1">
                <li>
                  <b>Múi giờ</b> — khung giờ bên dưới tính theo múi này. Quan trọng: lịch chạy theo
                  giờ máy chủ (UTC), nên &quot;6h–23h&quot; mà không nói múi giờ thì lệch 7 tiếng.
                </li>
                <li>
                  <b>Bắt đầu / Ngừng quét</b> — thu hẹp lại để khỏi quét lúc kênh chắc chắn không
                  đăng. Đặt giờ ngừng <i>sớm hơn</i> giờ bắt đầu = khung vắt qua nửa đêm
                  (22:00–06:00).
                </li>
                <li>
                  <b>Ngày trong tuần</b> — bỏ tick ngày kênh nghỉ. Bỏ tick hết cũng là quét mọi
                  ngày.
                </li>
              </ul>
              <Note>
                Đổi tần suất hoặc lịch có hiệu lực sau tối đa 30 giây — đó là chu kỳ hệ thống đọc
                lại lịch từ database.
              </Note>
            </SubSection>

            <SubSection id="dk-logic" title="2.4 Kênh quét ra bài như thế nào">
              <p>Đây là phần hay bị hiểu nhầm nhất, nên viết ra đúng thứ tự:</p>
              <Step n={1} title="Vòng quét ĐẦU TIÊN — lúc vừa thêm kênh">
                Hệ thống hỏi nền tảng lấy đúng <b>Số bài cũ của kênh</b> bài mới nhất (mới nhất
                trước, lùi dần về cũ), tạo Bài Post cho tất cả, rồi <b>ghi lại ID bài mới nhất</b>{" "}
                làm mốc đồng bộ.
              </Step>
              <Step n={2} title="Các vòng sau — theo lịch đã đặt">
                Hỏi nền tảng lấy <b>Số bài mỗi lần quét</b> bài mới nhất, rồi chỉ giữ những bài{" "}
                <i>mới hơn mốc</i>. Xử lý từ cũ đến mới để mốc tiến liên tục, xong thì cập nhật mốc
                thành bài mới nhất vừa lấy.
              </Step>
              <Step n={3} title="Tắt rồi bật lại kênh">
                Bật lại là <b>quét ngay</b>, không chờ hết chu kỳ. Nếu lúc bật đang nằm ngoài khung
                giờ của kênh thì vòng đó tự bỏ qua và chờ tới giờ theo lịch.
              </Step>
              <Note tone="warn">
                Hệ quả cần nhớ: <b>Số bài cũ của kênh</b> chỉ có tác dụng ở vòng đầu. Sau khi vòng
                đầu chạy xong, ô đó bị khoá lại vì đổi số cũng không còn gì để lấy nữa.
              </Note>
              <Tip title="Chống lấy trùng">
                Một bài chỉ vào hệ thống đúng một lần, tính theo <b>ID bài trên nền tảng</b> chứ
                không theo URL — cùng một video Facebook có cả dạng <code>/watch?v=</code> và{" "}
                <code>/reel/</code>, và cùng một bài nằm trong hai kênh vẫn chỉ tạo một Bài Post.
              </Tip>
            </SubSection>

            <SubSection id="dk-bang" title="2.5 Các cột trong bảng">
              <Table>
                <thead>
                  <tr>
                    <Th className="w-40">Cột</Th>
                    <Th>Đọc thế nào</Th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <Td className="font-medium text-slate-900">Kênh</Td>
                    <Td>URL kênh (bấm mở được) + nền tảng.</Td>
                  </tr>
                  <tr>
                    <Td className="font-medium text-slate-900">Tần suất</Td>
                    <Td>Khoảng nghỉ giữa hai vòng quét.</Td>
                  </tr>
                  <tr>
                    <Td className="font-medium text-slate-900">Hình thức / Ngôn ngữ</Td>
                    <Td>
                      Ngôn ngữ đổi thẳng trong ô chọn trên bảng, không cần mở hộp thoại — và đổi ở
                      đây <b>lan xuống</b> mọi Bài Post của kênh.
                    </Td>
                  </tr>
                  <tr>
                    <Td className="font-medium text-slate-900">Kết quả</Td>
                    <Td>
                      Số Bài Post và số Voice kênh này đã đẻ ra. Đã quét mà vẫn 0 nghĩa là kênh
                      chạy nhưng không bắt được gì.
                    </Td>
                  </tr>
                  <tr>
                    <Td className="font-medium text-slate-900">Thời gian</Td>
                    <Td>
                      Mốc tạo và mốc quét gần nhất. Dòng <span className="text-red-700">đỏ</span> là
                      lỗi của vòng quét gần nhất — không có dòng đó nghĩa là vòng vừa rồi chạy sạch.
                    </Td>
                  </tr>
                  <tr>
                    <Td className="font-medium text-slate-900">Cấu hình quét</Td>
                    <Td>Tóm tắt ba con số ở mục 2.2, lịch quét, và mốc đồng bộ hiện tại.</Td>
                  </tr>
                  <tr>
                    <Td className="font-medium text-slate-900">Trạng thái</Td>
                    <Td>Công tắc bật/tắt. Gạt là có hiệu lực ngay.</Td>
                  </tr>
                </tbody>
              </Table>
            </SubSection>

            <SubSection id="dk-sua" title="2.6 Sửa / bật tắt / xoá kênh">
              <ul className="ml-4 list-disc space-y-1">
                <li>
                  <b>Sửa</b> — nút trên từng dòng, mở lại đúng hộp thoại lúc thêm. Mọi trường đều
                  sửa được, trừ <i>Số bài cũ của kênh</i> sau khi vòng đầu đã chạy.
                </li>
                <li>
                  <b>Bật / tắt</b> — công tắc ở cột Trạng thái. Tắt thì hệ thống bỏ qua mọi vòng
                  quét; bật lại thì quét ngay.
                </li>
                <li>
                  <b>Xoá</b> — chỉ xoá kênh. Bài Post và Voice đã tạo vẫn còn.
                </li>
              </ul>
            </SubSection>
          </Section>

          {/* ------------------------------------------------------------ */}
          <Section id="kenh-breaking" title="3. Danh sách kênh Breaking">
            <SubSection id="bk-khacgi" title="3.1 Khác Định kỳ ở đâu">
              <p>
                <NavLink href="/lists/breaking">Mở màn Breaking</NavLink>. Cùng một hộp thoại, cùng
                các tham số quét, khác đúng hai điểm:
              </p>
              <ul className="ml-4 list-disc space-y-1">
                <li>
                  <b>Chỉ lấy bài khớp điều kiện</b> — bài không khớp bị bỏ qua hoàn toàn.
                </li>
                <li>
                  <b>Không có mốc đồng bộ</b> — mỗi vòng xét lại cùng một cửa sổ bài mới nhất, và
                  dựa vào chống-trùng để không tạo lại. Nên cửa sổ quét ở đây quan trọng hơn: đặt
                  quá nhỏ là bỏ sót bài.
                </li>
              </ul>
            </SubSection>

            <SubSection id="bk-regex" title="3.2 Điều kiện bắt bài">
              <p>
                Gõ từ khoá, hashtag hay biểu thức chính quy — mỗi dòng một điều kiện, khớp{" "}
                <b>một dòng bất kỳ</b> là lấy bài. Hệ thống tự chuẩn hoá từ khoá thô thành regex,
                nên không cần biết regex vẫn dùng được.
              </p>
              <Note>
                Điều kiện so với <b>toàn bộ phần chữ</b> của bài (tiêu đề + mô tả), không phân biệt
                hoa thường.
              </Note>
              <Tip title="Không bắt được bài nào?">
                Cột Kết quả vẫn 0 dù cột Thời gian cho thấy đã quét nghĩa là điều kiện không khớp.
                Bấm <b>Quét thử</b> để chạy một vòng ngay và xem lại.
              </Tip>
            </SubSection>
          </Section>

          {/* ------------------------------------------------------------ */}
          <Section id="tao-voice" title="4. Tạo Voice thủ công">
            <SubSection id="tv-hinhthuc" title="4.1 Ba hình thức tạo">
              <Table>
                <thead>
                  <tr>
                    <Th className="w-44">Hình thức</Th>
                    <Th>Làm gì</Th>
                    <Th>Dùng khi</Th>
                    <Th className="w-24">Trạng thái</Th>
                  </tr>
                </thead>
                <tbody>
                  {[
                    {
                      mode: "A",
                      name: "Extract từ URL",
                      what: "Tải video/audio gốc, tách thẳng giọng nói ra file voice",
                      when: "Muốn giữ nguyên giọng người trong video",
                    },
                    {
                      mode: "B",
                      name: "Text → TTS",
                      what: "AI đọc đúng đoạn chữ có sẵn (nội dung bài, hoặc chữ bạn gõ)",
                      when: "Bài chỉ có chữ, hoặc muốn giọng đọc thống nhất",
                    },
                    {
                      mode: "C",
                      name: "Text + Prompt → TTS",
                      what: "LLM viết lại nội dung theo Prompt mẫu (kèm tiêu đề + hashtag), rồi AI đọc bản viết lại",
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
              {modeMeta
                .filter((m) => !m.enabled && m.reason)
                .map((m) => (
                  <Note key={m.mode} tone="warn">
                    Hình thức <b>{m.mode}</b> đang tắt: {m.reason}
                  </Note>
                ))}

              <Tip title="Hình thức C: bản LLM viết ra mới là thứ được đọc">
                Đường đi là <i>nội dung nguồn + prompt → LLM → bản mới → TTS đọc bản mới</i>. Một
                lần gọi LLM trả về ba thứ: <b>tiêu đề</b>, <b>nội dung đọc</b> và <b>hashtag</b> —
                bạn không phải gõ tay. Thứ bạn đã tự điền thì hệ thống giữ nguyên.
              </Tip>
            </SubSection>

            <SubSection id="tv-form" title="4.2 Từng ô trong hộp thoại">
              <p>
                Ở màn <NavLink href="/voices">Voice</NavLink> bấm <b>+ Tạo Voice</b>. Một hộp thoại
                duy nhất cho cả ba hình thức — ô <b>Hình thức tạo</b> quyết định phần nhập nguồn,
                phần còn lại dùng chung.
              </p>
              <Table>
                <thead>
                  <tr>
                    <Th className="w-52">Ô</Th>
                    <Th>Nghĩa là gì</Th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <Td className="font-medium text-slate-900">Hình thức tạo</Td>
                    <Td>A / B / C — xem bảng ở mục 4.1.</Td>
                  </tr>
                  <tr>
                    <Td className="font-medium text-slate-900">Prompt mẫu + Bộ API</Td>
                    <Td>
                      Chỉ hiện với hình thức C. <b>Chọn sẵn theo lần bạn dùng gần nhất</b>, đổi lại
                      được. Bỏ trống Bộ API thì hệ thống rơi về key khai trong cấu hình server — ở
                      production chỗ đó không có key nào.
                    </Td>
                  </tr>
                  <tr>
                    <Td className="font-medium text-slate-900">URL bài đăng</Td>
                    <Td>
                      Chỉ hiện với hình thức A. Link một bài cụ thể (YouTube, Facebook, TikTok,
                      Instagram, X).
                    </Td>
                  </tr>
                  <tr>
                    <Td className="font-medium text-slate-900">Nội dung</Td>
                    <Td>
                      Hiện với hình thức B và C. Với <b>B</b> đây chính là lời TTS đọc. Với <b>C</b>{" "}
                      đây là <i>đầu vào</i> cho LLM — lời đọc thật là bản LLM viết lại.
                    </Td>
                  </tr>
                  <tr>
                    <Td className="font-medium text-slate-900">Tiêu đề</Td>
                    <Td>
                      Tối đa 200 ký tự. Bỏ trống thì lấy nội dung bài gốc (A) hoặc cắt từ nội dung
                      (B), hoặc lấy tiêu đề LLM đặt (C).
                    </Td>
                  </tr>
                  <tr>
                    <Td className="font-medium text-slate-900">Ngôn ngữ / Quốc gia</Td>
                    <Td>
                      Ngôn ngữ là tiếng TTS sẽ đọc. Quốc gia chỉ dùng để lọc danh sách tài khoản
                      đứng tên bài.
                    </Td>
                  </tr>
                  <tr>
                    <Td className="font-medium text-slate-900">Hashtag</Td>
                    <Td>
                      <b>Bắt buộc</b> với hình thức B và C: không có bài gốc nào để gộp thẻ vào, mà
                      multime từ chối bài không hashtag. Gõ để tìm trong danh mục MultiMe, hoặc gõ
                      tag mới rồi Enter.
                    </Td>
                  </tr>
                  <tr>
                    <Td className="font-medium text-slate-900">Tài khoản đứng tên</Td>
                    <Td>
                      Bắt buộc chọn giới tính. Việc bốc tài khoản cụ thể lùi tới lúc đăng, nên ở đây
                      chỉ cần nói ý muốn.
                    </Td>
                  </tr>
                  <tr>
                    <Td className="font-medium text-slate-900">Ảnh bìa</Td>
                    <Td>
                      Tải ảnh từ máy, hoặc tick <b>Lấy ảnh từ nguồn</b> (chỉ hình thức A — text gõ
                      tay không có bài nào để lấy ảnh). Không chọn gì = bài không ảnh, vẫn đăng
                      được.
                    </Td>
                  </tr>
                </tbody>
              </Table>
              <Note>
                Bấm <b>Đăng</b> là hộp thoại đóng ngay. Hệ thống tạo audio rồi <b>tự đăng</b> lên
                multime khi xong — không có bước bấm nút thứ hai.
              </Note>
            </SubSection>
          </Section>

          {/* ------------------------------------------------------------ */}
          <Section id="man-voice" title="5. Màn Voice">
            <SubSection id="v-bang" title="5.1 Các cột và trạng thái">
              <p>
                <NavLink href="/voices">Mở màn Voice</NavLink>. Đây là nơi nghe thử, sửa và đăng.
              </p>
              <ul className="ml-4 list-disc space-y-1">
                <li>
                  <b>Đang xử lý</b> — worker đang tạo audio. Chờ, không bấm gì.
                </li>
                <li>
                  <b>Nháp</b> — đã có file, đủ điều kiện đăng.
                </li>
                <li>
                  <b>Chưa đủ điều kiện</b> — thiếu tiêu đề, hashtag, tác giả, hoặc audio ngắn hơn
                  mức multime nhận. Mở ra điền nốt.
                </li>
                <li>
                  <b>Lỗi</b> — có dòng lý do ngay dưới trạng thái. Sửa xong thì tạo lại.
                </li>
                <li>
                  <b>Đã đăng</b> — có link bài trên multime. Voice đã đăng thì không sửa được nữa.
                </li>
              </ul>
            </SubSection>

            <SubSection id="v-thongtin" title="5.2 Sửa tab Thông tin">
              <p>
                Sửa phần chữ đi kèm bài đăng: tiêu đề, hashtag, ảnh bìa, tác giả, ngôn ngữ.{" "}
                <b>Không đụng tới file audio</b> — bấm Lưu không đọc lại gì cả.
              </p>
            </SubSection>

            <SubSection id="v-noidung" title="5.3 Sửa tab Nội dung">
              <p>
                Tab này <b>đọc lại</b> và ghi đè file audio cũ. Với hình thức C có hai ô, và chúng
                khác nhau:
              </p>
              <ul className="ml-4 list-disc space-y-1">
                <li>
                  <b>Nội dung</b> — đầu vào của Prompt mẫu. Sửa ở đây rồi bấm Tạo lại là{" "}
                  <i>chạy prompt lần nữa</i>, lời đọc cũ bị thay.
                </li>
                <li>
                  <b>Nội dung đọc</b> — đúng đoạn TTS đã đọc ra file hiện tại (bản LLM viết lại).
                  Sửa tay ở đây thì lần tạo lại <i>đọc nguyên văn</i>, không gọi LLM nữa.
                </li>
              </ul>
              <Note>
                Form in sẵn một dòng ngay trên nút bấm nói rõ cái sắp xảy ra, nên không phải đoán.
                Hình thức B chỉ có một ô — nó vừa là nguồn vừa là lời đọc.
              </Note>
            </SubSection>

            <SubSection id="v-dang" title="5.4 Đăng lên multime">
              <p>
                Voice tạo từ hộp thoại <b>+ Tạo Voice</b> và voice từ kênh bật tự động đều{" "}
                <b>tự đăng</b>. Còn lại thì bấm <b>Đăng</b> trên từng dòng.
              </p>
              <Note tone="warn">
                Voice ngắn hơn mức tối thiểu của multime sẽ bị từ chối. Đăng bằng tài khoản
                strongbody của chính bạn, dưới tên tác giả đã chọn.
              </Note>
            </SubSection>
          </Section>

          {/* ------------------------------------------------------------ */}
          <Section id="man-baipost" title="6. Màn Bài Post">
            <p>
              <NavLink href="/source-posts">Mở màn Bài Post</NavLink>. Đây là chỗ duyệt bài lấy từ
              kênh <i>trước khi</i> tốn chi phí AI — đặc biệt hữu ích khi kênh tắt chế độ tự động.
            </p>
            <ul className="ml-4 list-disc space-y-1">
              <li>
                Cột <b>Danh sách kênh</b> cho biết bài đến từ kênh nào; bài nhập tay hiện{" "}
                <i>Nhập tay</i>.
              </li>
              <li>
                <b>Hình thức</b> và <b>Ngôn ngữ</b> đổi thẳng trong ô chọn trên bảng. Đổi ngôn ngữ
                của bài lan xuống các voice chưa tạo xong file của chính bài đó.
              </li>
              <li>
                <b>Chạy tạo Voice</b> — bấm để tạo voice cho bài đó. Bài lỗi thì nút thành{" "}
                <i>Chạy lại Voice</i>.
              </li>
              <li>
                Chọn nhiều dòng để đổi hình thức / ngôn ngữ / chạy voice hàng loạt.
              </li>
            </ul>
            <Note>
              Metadata gốc (ảnh bìa, tác giả, ngày đăng, hashtag) được lấy tự động ngay khi bài vào
              hệ thống, kể cả bài từ kênh — không phải chờ tới lúc tạo voice.
            </Note>
          </Section>

          {/* ------------------------------------------------------------ */}
          <Section id="danh-muc" title="7. Prompt mẫu & AI Engine">
            <SubSection id="dm-prompt" title="7.1 Prompt mẫu">
              <p>
                <NavLink href="/prompts">Mở màn Prompt mẫu</NavLink>. Đây là chỉ dẫn biên tập cho
                hình thức C: mô tả rõ độ dài, giọng điệu, những gì cần bỏ.
              </p>
              <ul className="ml-4 list-disc space-y-1">
                <li>
                  <b>Thêm</b> — form bên trái.
                </li>
                <li>
                  <b>Sửa</b> — nút trên từng dòng. Sửa tại chỗ chứ không xoá rồi thêm lại: prompt
                  đang được các kênh và voice trỏ tới.
                </li>
                <li>
                  Sửa nội dung <b>không đụng</b> voice đã tạo — chúng đã đọc xong bằng bản cũ. Bản
                  mới áp dụng từ lần chạy kế tiếp.
                </li>
              </ul>
              <Note>
                Không cần viết &quot;chỉ trả về kịch bản&quot; hay yêu cầu định dạng JSON — hệ thống
                đã bọc sẵn phần đó.
              </Note>
            </SubSection>

            <SubSection id="dm-tts" title="7.2 API key TTS">
              <p>
                <NavLink href="/ai-engines">AI Engine</NavLink> → tab <b>TTS</b>. Mỗi người tự khai
                key 3voices của mình; hạn mức và hoá đơn rơi đúng vào người dùng nó.
              </p>
              <Note tone="warn">
                Bắt buộc với hình thức B và C. Chưa khai key thì voice chạy tới bước đọc rồi báo
                lỗi kèm câu hướng dẫn.
              </Note>
            </SubSection>

            <SubSection id="dm-llm" title="7.3 Bộ API key LLM">
              <p>
                <NavLink href="/ai-engines">AI Engine</NavLink> → tab <b>LLM Model</b> →{" "}
                <b>Thêm bộ API</b>. Chỉ cần nếu dùng hình thức C.
              </p>
              <ul className="ml-4 list-disc space-y-1">
                <li>
                  Một <b>bộ</b> là túi key của nhiều nhà (Gemini, OpenAI, Anthropic), không phải một
                  key lẻ. Hệ thống thử lần lượt từ model rẻ nhất, hết hạn mức thì tự chuyển sang nhà
                  kế tiếp.
                </li>
                <li>
                  Cột <b>Sức khoẻ</b> nói luôn phải làm gì: <i>Đang nghỉ</i> = hết hạn mức, chờ tới
                  giờ ghi trong đó; <i>Đã tắt</i> = key sai hoặc bị thu hồi, phải dán key mới.
                </li>
                <li>
                  Bộ dùng chung được qua ô <b>Dùng chung với</b>. Người được chia dùng được nhưng
                  không sửa được key.
                  {isAdmin ? (
                    <>
                      {" "}
                      Là admin, bạn còn bật được <b>Hiện với mọi người</b> — bật lên là mở hạn mức
                      và chi phí của bộ đó cho cả hệ thống.
                    </>
                  ) : null}
                </li>
              </ul>
              <Note tone="warn">
                Model bị nhà cung cấp gỡ khỏi API thì key vẫn &quot;sống&quot; nhưng mỗi lần chạy
                mất vài giây rơi qua nó. Thấy voice hình thức C chậm bất thường thì kiểm tra lại tên
                model trong bộ.
              </Note>
            </SubSection>
          </Section>

          {/* ------------------------------------------------------------ */}
          <Section id="su-co" title="8. Gặp sự cố thì xem ở đâu">
            <Table>
              <thead>
                <tr>
                  <Th className="w-64">Triệu chứng</Th>
                  <Th>Xem ở đâu</Th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <Td className="font-medium text-slate-900">Kênh không ra bài nào</Td>
                  <Td>
                    Cột <b>Thời gian</b> ở bảng kênh: có dòng đỏ là lỗi vòng quét; không có dòng đỏ
                    mà cột Kết quả vẫn 0 thì kênh chạy đúng nhưng chưa có bài mới (hoặc regex không
                    khớp với kênh Breaking).
                  </Td>
                </tr>
                <tr>
                  <Td className="font-medium text-slate-900">Voice kẹt ở &quot;Đang xử lý&quot;</Td>
                  <Td>Chờ thêm; quá lâu thì kiểm tra lại key TTS còn hạn mức không.</Td>
                </tr>
                <tr>
                  <Td className="font-medium text-slate-900">Voice báo lỗi</Td>
                  <Td>
                    Dòng lý do nằm ngay dưới trạng thái ở màn Voice. Sửa nguyên nhân rồi bấm tạo
                    lại.
                  </Td>
                </tr>
                <tr>
                  <Td className="font-medium text-slate-900">Voice không đăng được</Td>
                  <Td>
                    Trạng thái <b>Chưa đủ điều kiện</b> = thiếu tiêu đề / hashtag / tác giả, hoặc
                    audio quá ngắn. Mở tab Thông tin điền nốt.
                  </Td>
                </tr>
                <tr>
                  <Td className="font-medium text-slate-900">Hình thức C bị mờ</Td>
                  <Td>
                    Chưa có Bộ API key LLM nào. Thêm một bộ ở AI Engine → LLM Model là nó bật ngay,
                    không cần khởi động lại.
                  </Td>
                </tr>
                <tr>
                  <Td className="font-medium text-slate-900">Ai đã đổi cái gì</Td>
                  <Td>
                    <NavLink href="/audit-log">Nhật ký thao tác</NavLink> ghi lại mọi thay đổi kèm
                    người thực hiện.
                  </Td>
                </tr>
              </tbody>
            </Table>
          </Section>
        </div>

        {/* Mục lục dính theo màn hình: trang này dài, và thứ người ta cần gần
            như luôn là một mục cụ thể chứ không phải đọc từ đầu. */}
        <aside className="space-y-6 xl:sticky xl:top-6 xl:self-start">
          <Card>
            <CardHeader title="Mục lục" />
            <CardBody className="space-y-1 text-sm">
              {TOC.map((item) => (
                <div key={item.id}>
                  <a
                    href={`#${item.id}`}
                    className="block rounded px-2 py-1 font-medium text-slate-800 hover:bg-slate-50 hover:text-indigo-700"
                  >
                    {item.label}
                  </a>
                  {item.sub?.map((s) => (
                    <a
                      key={s.id}
                      href={`#${s.id}`}
                      className="block rounded px-2 py-0.5 pl-5 text-slate-600 hover:bg-slate-50 hover:text-indigo-700"
                    >
                      {s.label}
                    </a>
                  ))}
                </div>
              ))}
            </CardBody>
          </Card>

          <Card>
            <CardHeader title="Quyền của bạn" description={me.data?.role ?? "…"} />
            <CardBody className="space-y-1 text-sm">
              <Perm ok label="Xem mọi thứ" />
              <Perm ok label="Tạo / sửa / chạy / đăng" />
              <Perm ok={me.data?.role !== "user"} label="Xoá" />
              <Perm ok={isAdmin} label="Quản lý tài khoản & cài đặt" />
            </CardBody>
          </Card>
        </aside>
      </div>
    </>
  );
}

// ---------------------------------------------------------------------------
// Thành phần trình bày
// ---------------------------------------------------------------------------

function Section({
  id,
  title,
  children,
}: {
  id: string;
  title: string;
  children: React.ReactNode;
}) {
  return (
    // scroll-mt để tiêu đề không bị dính sát mép trên khi nhảy từ mục lục.
    <div id={id} className="scroll-mt-6">
      <Card>
        <CardHeader title={title} />
        <CardBody className="space-y-4 text-sm text-slate-700">{children}</CardBody>
      </Card>
    </div>
  );
}

function SubSection({
  id,
  title,
  children,
}: {
  id: string;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <section id={id} className="scroll-mt-6 space-y-3 border-t border-slate-100 pt-4 first:border-0 first:pt-0">
      <h3 className="font-semibold text-slate-900">{title}</h3>
      {children}
    </section>
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

function Tip({ id, title, children }: { id?: string; title: string; children: React.ReactNode }) {
  return (
    <div id={id} className="scroll-mt-6 rounded-md border border-slate-200 p-3">
      <p className="font-medium text-slate-900">{title}</p>
      <div className="mt-1 text-slate-600">{children}</div>
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

function Perm({ ok, label }: { ok?: boolean; label: string }) {
  return (
    <div className="flex items-center gap-2">
      <span className={ok ? "text-green-700" : "text-slate-400"}>{ok ? "✓" : "✗"}</span>
      <span className={ok ? "" : "text-slate-400"}>{label}</span>
    </div>
  );
}
