import { useEffect } from "react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

interface ConfirmDialogProps {
  open: boolean
  title: string
  description?: React.ReactNode
  confirmText?: string
  cancelText?: string
  /** 确认按钮用醒目的危险色（中断、删除这类操作） */
  danger?: boolean
  onConfirm: () => void
  onCancel: () => void
}

/**
 * 通用二次确认弹窗。
 * 支持 Esc 取消、点遮罩取消、回车确认。
 */
export function ConfirmDialog({
  open,
  title,
  description,
  confirmText = "确定",
  cancelText = "取消",
  danger,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onCancel()
      if (e.key === "Enter") onConfirm()
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [open, onConfirm, onCancel])

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-[100] flex items-center justify-center bg-black/40 backdrop-blur-[2px]"
      onClick={onCancel}
    >
      <div
        className="mx-4 w-full max-w-sm rounded-xl border border-border bg-background p-5 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="text-base font-semibold">{title}</h3>
        {description && (
          <div className="mt-2 text-sm text-muted-foreground">{description}</div>
        )}
        <div className="mt-5 flex justify-end gap-2">
          <Button size="sm" variant="outline" onClick={onCancel}>
            {cancelText}
          </Button>
          <Button
            size="sm"
            onClick={onConfirm}
            className={cn(danger && "bg-destructive text-destructive-foreground hover:bg-destructive/90")}
          >
            {confirmText}
          </Button>
        </div>
      </div>
    </div>
  )
}
