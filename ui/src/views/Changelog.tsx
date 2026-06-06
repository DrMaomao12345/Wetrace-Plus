import { useQuery } from "@tanstack/react-query"
import { systemApi } from "@/api"
import { ScrollArea } from "@/components/ui/scroll-area"

// Render a subset of Markdown used in CHANGELOG.md without external deps.
// Handles: ---  ## ### - **bold** plain text
function renderMarkdown(md: string): React.ReactNode[] {
  const nodes: React.ReactNode[] = []
  const lines = md.split("\n")
  let i = 0

  const renderInline = (text: string): React.ReactNode[] => {
    const parts = text.split(/(\*\*[^*]+\*\*)/)
    return parts.map((part, idx) => {
      if (part.startsWith("**") && part.endsWith("**")) {
        return <strong key={idx}>{part.slice(2, -2)}</strong>
      }
      return part
    })
  }

  while (i < lines.length) {
    const line = lines[i]

    if (line.trim() === "---") {
      nodes.push(<hr key={i} className="border-border my-6" />)
    } else if (line.startsWith("## ")) {
      nodes.push(
        <h2 key={i} className="text-xl font-bold text-foreground mt-8 mb-3 first:mt-0">
          {line.slice(3)}
        </h2>
      )
    } else if (line.startsWith("### ")) {
      nodes.push(
        <h3 key={i} className="text-sm font-semibold text-muted-foreground uppercase tracking-wider mt-5 mb-2">
          {line.slice(4)}
        </h3>
      )
    } else if (line.startsWith("- ")) {
      nodes.push(
        <div key={i} className="flex gap-2 text-sm text-foreground/90 leading-relaxed py-0.5">
          <span className="text-muted-foreground shrink-0 mt-0.5">•</span>
          <span>{renderInline(line.slice(2))}</span>
        </div>
      )
    } else if (line.trim() !== "") {
      nodes.push(
        <p key={i} className="text-sm text-foreground/80 leading-relaxed">
          {renderInline(line)}
        </p>
      )
    }

    i++
  }

  return nodes
}

export default function Changelog() {
  const { data, isLoading } = useQuery({
    queryKey: ["changelog"],
    queryFn: () => systemApi.getChangelog(),
    staleTime: Infinity,
  })

  return (
    <div className="h-full flex flex-col">
      <div className="px-6 py-4 border-b border-border shrink-0">
        <h1 className="text-xl font-bold">更新日志</h1>
        <p className="text-sm text-muted-foreground mt-0.5">版本历史与变更记录</p>
      </div>

      <ScrollArea className="flex-1">
        <div className="max-w-2xl mx-auto px-6 py-6">
          {isLoading && (
            <p className="text-sm text-muted-foreground">加载中…</p>
          )}
          {data?.content && renderMarkdown(data.content)}
        </div>
      </ScrollArea>
    </div>
  )
}
