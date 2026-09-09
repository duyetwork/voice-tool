"use client";

import { audioUrl } from "@/lib/api";

/**
 * AudioPreview — thanh phát file voice, hiện sẵn ngay trong bảng.
 *
 * File voice nằm trong bucket riêng tư và host storage (`minio:9000`) chỉ tồn
 * tại trong mạng Docker, nên không gắn thẳng `voice_file_url` vào <audio> được;
 * ở đây trỏ vào endpoint của API. Thẻ <audio> không gắn được header
 * Authorization nên token đi kèm trong query (xem `middleware.AuthMedia`).
 *
 * `preload="none"`: trình duyệt chỉ tải file khi người dùng bấm play — mở bảng
 * 50 voice không kéo về 50 file. Nút tải về nằm sẵn trong menu của thanh phát.
 */
export function AudioPreview({ voiceId }: { voiceId: string }) {
  return <audio controls preload="none" src={audioUrl(voiceId)} className="mt-2 h-9 w-full max-w-md" />;
}
