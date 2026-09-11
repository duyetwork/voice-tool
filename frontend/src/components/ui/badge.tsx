import { cn } from "@/lib/utils";

const TONES: Record<string, string> = {
  neutral: "bg-slate-100 text-slate-700",
  info: "bg-sky-100 text-sky-800",
  success: "bg-green-100 text-green-800",
  warning: "bg-amber-100 text-amber-800",
  danger: "bg-red-100 text-red-800",
};

/** statusTone map trạng thái của Bài Post / Voice sang màu nhãn. */
export function statusTone(status: string): keyof typeof TONES {
  switch (status) {
    case "processed":
    case "published":
    case "active":
      return "success";
    case "processing":
    case "ready":
      return "info";
    case "failed":
      return "danger";
    case "paused":
    case "draft":
    // Chưa đủ điều kiện đăng: nhắc người dùng điền nốt, không phải lỗi hệ thống.
    case "incomplete":
      return "warning";
    default:
      return "neutral";
  }
}

export function Badge({
  children,
  tone = "neutral",
  className,
}: {
  children: React.ReactNode;
  tone?: keyof typeof TONES;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium",
        TONES[tone],
        className,
      )}
    >
      {children}
    </span>
  );
}
