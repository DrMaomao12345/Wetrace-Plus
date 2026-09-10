import { Outlet } from "react-router-dom"
import { Sidebar } from "./Sidebar"
import { MobileNav } from "./MobileNav"
import { useAppStore } from "@/stores/app"
import { cn } from "@/lib/utils"

export function MainLayout() {
  const isMobile = useAppStore((state) => state.isMobile)

  return (
    <div
      className={cn(
        "flex h-screen h-dvh w-full overflow-hidden bg-background text-foreground",
        isMobile ? "flex-col" : "flex-row",
      )}
    >
      {!isMobile && <Sidebar />}
      <main className="relative flex min-h-0 min-w-0 flex-1 flex-col overflow-y-auto overscroll-contain">
        <Outlet />
      </main>
      {isMobile && <MobileNav />}
    </div>
  )
}
