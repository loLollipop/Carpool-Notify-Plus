import { cn } from "@/lib/utils"

export function EmailPreview({
  html,
  plainText,
  title,
  className,
}: {
  html?: string
  plainText: string
  title: string
  className?: string
}) {
  if (html?.trim()) {
    return (
      <div
        className={cn(
          "overflow-hidden rounded-xl border bg-[#f3f7f6] shadow-[0_8px_24px_rgba(24,63,55,0.07)]",
          className,
        )}
      >
        <iframe
          title={title}
          srcDoc={html}
          sandbox=""
          className="h-[420px] w-full border-0 bg-[#f3f7f6]"
        />
      </div>
    )
  }

  return (
    <pre
      className={cn(
        "max-h-80 overflow-auto rounded-xl border bg-muted/35 p-4 font-sans text-[13px] leading-6 whitespace-pre-wrap",
        className,
      )}
    >
      {plainText}
    </pre>
  )
}
