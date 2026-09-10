import { readFileSync, writeFileSync } from "node:fs"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

const here = dirname(fileURLToPath(import.meta.url))
const root = join(here, "..", "..")
const version = readFileSync(join(root, "VERSION"), "utf8").trim()

if (!/^\d+\.\d+\.\d+$/.test(version)) {
  console.error(`VERSION 内容不合法：${JSON.stringify(version)}，应形如 1.0.0`)
  process.exit(1)
}

const target = join(here, "..", "src", "version.ts")
const current = readFileSync(target, "utf8")
const next = current.replace(/export const APP_VERSION = "v[^"]*"/, `export const APP_VERSION = "v${version}"`)

if (next !== current) {
  writeFileSync(target, next)
  console.log(`版本号已同步：v${version}`)
} else {
  console.log(`版本号已是最新：v${version}`)
}
