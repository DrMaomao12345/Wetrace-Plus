import { useRef, useState } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import {
  AlertCircle,
  Archive,
  CheckCircle2,
  FileJson,
  FileSpreadsheet,
  History,
  Loader2,
  ShieldCheck,
  UploadCloud,
  X,
} from "lucide-react"
import { toast } from "sonner"

import { importApi, type ImportResult } from "@/api/imports"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

const acceptedExtensions = [".json", ".csv", ".zip"]

function supported(file: File) {
  const lower = file.name.toLowerCase()
  return acceptedExtensions.some((extension) => lower.endsWith(extension))
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}

function formatName(id: string) {
  const labels: Record<string, string> = {
    "chatlog-keeper-json": "chatlog-keeper 微信 JSON",
    "chatlog-json": "chatlog JSON",
    "chatlog-csv": "chatlog CSV",
    "memotrace-csv": "MemoTrace CSV",
    "memotrace-full-csv": "MemoTrace 全量 CSV",
    "wetrace-plus-json": "Wetrace Plus JSON",
  }
  return labels[id] || id
}

export default function ImportView() {
  const queryClient = useQueryClient()
  const fileInput = useRef<HTMLInputElement>(null)
  const [files, setFiles] = useState<File[]>([])
  const [selfId, setSelfId] = useState("")
  const [selfName, setSelfName] = useState("")
  const [dragging, setDragging] = useState(false)
  const [importing, setImporting] = useState(false)
  const [progress, setProgress] = useState(0)
  const [result, setResult] = useState<ImportResult | null>(null)

  const { data: formats = [] } = useQuery({
    queryKey: ["import-formats"],
    queryFn: importApi.getFormats,
    staleTime: Infinity,
  })
  const { data: history = [] } = useQuery({
    queryKey: ["import-history"],
    queryFn: () => importApi.getHistory(12),
  })

  const addFiles = (incoming: File[]) => {
    const invalid = incoming.filter((file) => !supported(file))
    if (invalid.length) toast.error(`不支持 ${invalid.map((file) => file.name).join("、")}`)
    setFiles((current) => {
      const byKey = new Map(current.map((file) => [`${file.name}:${file.size}:${file.lastModified}`, file]))
      incoming.filter(supported).forEach((file) => byKey.set(`${file.name}:${file.size}:${file.lastModified}`, file))
      return [...byKey.values()]
    })
    setResult(null)
  }

  const removeFile = (target: File) => {
    setFiles((current) => current.filter((file) => file !== target))
  }

  const startImport = async () => {
    if (!files.length) {
      toast.error("请先选择导出文件")
      return
    }
    setImporting(true)
    setProgress(0)
    setResult(null)
    try {
      const imported = await importApi.upload(files, { selfId, selfName }, setProgress)
      setResult(imported)
      setFiles([])
      setProgress(100)
      toast.success(`已导入 ${imported.imported.toLocaleString()} 条消息`)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["import-history"] }),
        queryClient.invalidateQueries({ queryKey: ["sessions"] }),
        queryClient.invalidateQueries({ queryKey: ["dashboard"] }),
      ])
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "导入失败")
    } finally {
      setImporting(false)
    }
  }

  return (
    <div className="mx-auto w-full max-w-6xl space-y-6 p-4 md:p-8">
      <div className="space-y-2">
        <div className="flex items-center gap-2 text-primary">
          <Archive className="h-5 w-5" />
          <span className="text-sm font-medium">Wetrace Plus 数据入口</span>
        </div>
        <h1 className="text-2xl font-semibold tracking-tight">导入聊天记录</h1>
        <p className="max-w-3xl text-sm text-muted-foreground">
          这里只接收其他工具已经导出的文件。Wetrace Plus 不连接微信、不读取微信进程，也不提取或解密微信数据库。
        </p>
      </div>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1.45fr)_minmax(300px,0.75fr)]">
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">选择导出文件</CardTitle>
            <CardDescription>可同时选择多个 JSON、CSV，或直接上传包含这些文件的 ZIP。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-5">
            <input
              ref={fileInput}
              className="hidden"
              type="file"
              multiple
              accept=".json,.csv,.zip,application/json,text/csv,application/zip"
              onChange={(event) => {
                addFiles(Array.from(event.target.files || []))
                event.target.value = ""
              }}
            />
            <button
              type="button"
              onClick={() => fileInput.current?.click()}
              onDragEnter={(event) => {
                event.preventDefault()
                setDragging(true)
              }}
              onDragOver={(event) => event.preventDefault()}
              onDragLeave={(event) => {
                event.preventDefault()
                setDragging(false)
              }}
              onDrop={(event) => {
                event.preventDefault()
                setDragging(false)
                addFiles(Array.from(event.dataTransfer.files))
              }}
              className={cn(
                "flex min-h-44 w-full flex-col items-center justify-center rounded-xl border-2 border-dashed px-6 py-8 text-center transition-colors",
                dragging ? "border-primary bg-primary/5" : "border-border hover:border-primary/50 hover:bg-muted/40",
              )}
            >
              <UploadCloud className="mb-3 h-9 w-9 text-primary" />
              <span className="font-medium">拖放文件到这里，或点击选择</span>
              <span className="mt-1 text-xs text-muted-foreground">JSON · CSV · ZIP，单次最多 2 GiB</span>
            </button>

            {files.length > 0 && (
              <div className="space-y-2">
                {files.map((file) => (
                  <div key={`${file.name}:${file.size}:${file.lastModified}`} className="flex items-center gap-3 rounded-lg border bg-muted/20 px-3 py-2.5">
                    {file.name.toLowerCase().endsWith(".json") ? (
                      <FileJson className="h-5 w-5 shrink-0 text-amber-500" />
                    ) : file.name.toLowerCase().endsWith(".csv") ? (
                      <FileSpreadsheet className="h-5 w-5 shrink-0 text-emerald-500" />
                    ) : (
                      <Archive className="h-5 w-5 shrink-0 text-blue-500" />
                    )}
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{file.name}</p>
                      <p className="text-xs text-muted-foreground">{formatBytes(file.size)}</p>
                    </div>
                    <button type="button" onClick={() => removeFile(file)} disabled={importing} className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground">
                      <X className="h-4 w-4" />
                    </button>
                  </div>
                ))}
              </div>
            )}

            <div className="grid gap-4 rounded-lg border bg-muted/20 p-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <label className="text-sm font-medium">你的微信 ID（可选）</label>
                <Input value={selfId} onChange={(event) => setSelfId(event.target.value)} placeholder="例如 wxid_xxx" disabled={importing} />
              </div>
              <div className="space-y-1.5">
                <label className="text-sm font-medium">你的导出昵称（建议填写）</label>
                <Input value={selfName} onChange={(event) => setSelfName(event.target.value)} placeholder="需与 CSV 中的发送人一致" disabled={importing} />
              </div>
              <p className="text-xs leading-relaxed text-muted-foreground sm:col-span-2">
                chatlog JSON 和带 IsSender 的全量 CSV 会直接使用原始方向；旧版会话 CSV 尤其是群聊没有方向字段，填写昵称后才能准确区分发送与接收。
              </p>
            </div>

            {importing && (
              <div className="space-y-2">
                <div className="flex justify-between text-xs text-muted-foreground">
                  <span>{progress < 100 ? "正在上传…" : "上传完成，正在校验并建立分析索引…"}</span>
                  <span>{progress}%</span>
                </div>
                <div className="h-2 overflow-hidden rounded-full bg-muted">
                  <div className="h-full bg-primary transition-all" style={{ width: `${Math.max(progress, 3)}%` }} />
                </div>
              </div>
            )}

            <Button className="w-full sm:w-auto" onClick={startImport} disabled={importing || files.length === 0}>
              {importing ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <UploadCloud className="mr-2 h-4 w-4" />}
              {importing ? "正在导入" : "导入并开始分析"}
            </Button>
          </CardContent>
        </Card>

        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base"><ShieldCheck className="h-4 w-4 text-emerald-500" />隐私边界</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2 text-sm text-muted-foreground">
              <p>文件仅在本机解析并写入分析库。</p>
              <p>不访问微信安装目录、进程内存或加密数据库。</p>
              <p>重复导入按消息内容指纹自动去重。</p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">已支持格式</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {formats.map((format) => (
                <div key={format.id} className="space-y-1 border-b pb-3 last:border-0 last:pb-0">
                  <div className="flex items-center justify-between gap-3">
                    <p className="text-sm font-medium">{format.name}</p>
                    <span className="text-[10px] uppercase text-muted-foreground">{format.extensions.join(" ")}</span>
                  </div>
                  <p className="text-xs leading-relaxed text-muted-foreground">{format.description}</p>
                </div>
              ))}
            </CardContent>
          </Card>
        </div>
      </div>

      {result && (
        <Card className="border-emerald-500/30 bg-emerald-500/[0.03]">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-lg"><CheckCircle2 className="h-5 w-5 text-emerald-500" />导入完成</CardTitle>
            <CardDescription>数据已经进入统一分析库，可以前往聊天、年度报告或关系洞察。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
              {[
                ["会话", result.conversations],
                ["识别消息", result.messages],
                ["新增", result.imported],
                ["重复跳过", result.duplicates],
              ].map(([label, value]) => (
                <div key={label} className="rounded-lg border bg-background p-3">
                  <p className="text-xs text-muted-foreground">{label}</p>
                  <p className="mt-1 text-xl font-semibold">{Number(value).toLocaleString()}</p>
                </div>
              ))}
            </div>
            <div className="space-y-2">
              {result.sources.map((source) => (
                <div key={source.file} className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-md bg-background px-3 py-2 text-sm">
                  <span className="min-w-0 flex-1 truncate font-medium">{source.file}</span>
                  <span className="text-xs text-muted-foreground">{formatName(source.format)}</span>
                  <span className="text-xs text-muted-foreground">新增 {source.imported.toLocaleString()}</span>
                </div>
              ))}
            </div>
            {!!result.warnings?.length && (
              <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-3">
                <p className="mb-2 flex items-center gap-2 text-sm font-medium text-amber-700 dark:text-amber-400"><AlertCircle className="h-4 w-4" />需要留意</p>
                <ul className="space-y-1 text-xs text-muted-foreground">
                  {result.warnings.slice(0, 8).map((warning, index) => (
                    <li key={`${warning.file}:${warning.row}:${index}`}>{warning.file}{warning.row ? ` 第 ${warning.row} 行` : ""}：{warning.message}</li>
                  ))}
                </ul>
              </div>
            )}
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base"><History className="h-4 w-4" />最近导入</CardTitle>
          <CardDescription>保留文件指纹与统计结果，不保存上传文件的额外副本。</CardDescription>
        </CardHeader>
        <CardContent>
          {history.length === 0 ? (
            <p className="py-4 text-center text-sm text-muted-foreground">还没有导入记录</p>
          ) : (
            <div className="divide-y">
              {history.map((item) => (
                <div key={item.id} className="flex flex-wrap items-center gap-x-4 gap-y-1 py-3 text-sm">
                  <div className="min-w-52 flex-1">
                    <p className="truncate font-medium">{item.source_name}</p>
                    <p className="text-xs text-muted-foreground">{new Date(item.imported_at).toLocaleString()}</p>
                  </div>
                  <span className="text-xs text-muted-foreground">{item.formats.split(",").map(formatName).join(" · ")}</span>
                  <span className="text-xs">新增 {item.imported.toLocaleString()}</span>
                  {item.duplicates > 0 && <span className="text-xs text-muted-foreground">跳过 {item.duplicates.toLocaleString()}</span>}
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
