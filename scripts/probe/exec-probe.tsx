/** @jsxImportSource @opentui/solid */
// Probe: does an OpenCode TUI plugin run an executable on this OS, and does a console window flash?
import type { TuiPlugin, TuiPluginModule } from "@opencode-ai/plugin/tui"
import { createSignal } from "solid-js"
import { execFile } from "node:child_process"
import { appendFileSync } from "node:fs"

const tui: TuiPlugin = async (api) => {
  const [out, setOut] = createSignal("probe: waiting")
  let n = 0
  const run = () => {
    const win = process.platform === "win32"
    const t0 = Date.now()
    execFile(win ? "cmd.exe" : "echo", win ? ["/c", "echo", "probe-ok"] : ["probe-ok"], { timeout: 10_000, windowsHide: true }, (err, stdout) => {
      n++
      const line = `${new Date().toISOString()} run=${n} ms=${Date.now() - t0} err=${err ? err.message : "none"} out=${stdout.trim()}`
      try { appendFileSync(process.env.PROBE_LOG ?? "exec-probe.log", line + "\n") } catch {}
      setOut(`probe #${n}: ${err ? "FAILED " + err.message : stdout.trim()}`)
    })
  }
  run()
  const tick = setInterval(run, 2000)
  api.lifecycle.onDispose(() => clearInterval(tick))
  api.slots.register({ slots: { app_bottom: () => <text>{out()}</text> } })
}

export default { id: "probe.exec", tui } satisfies TuiPluginModule & { id: string }
