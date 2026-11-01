/** @jsxImportSource @opentui/solid */
// Rainbow Statusline for OpenCode: collects the session from the TUI API, hands it to the
// rainbow-statusline binary and draws what comes back. All numbers are worked out in the binary.
import type { TuiPlugin, TuiPluginModule } from "@opencode-ai/plugin/tui"
import type { AssistantMessage } from "@opencode-ai/sdk/v2"
import { createSignal, For } from "solid-js"
import { execFile } from "node:child_process"

type Segment = { text: string; fg: string; bold: boolean; dim: boolean }

const BIN = process.env.RAINBOW_STATUSLINE_BIN || "rainbow-statusline"
const INTERVAL_MS = 1000
const LIMIT_MS = 2000
const DIM = "#6c6c6c"
const BASIC: Record<string, string> = { "31": "#d75f5f", "32": "#5faf5f", "33": "#d7af00", "34": "#5f87d7" }

// Maps the binary's SGR colour to what OpenTUI draws; undefined leaves the theme's colour.
function colour(fg: string): string | undefined {
  if (BASIC[fg]) return BASIC[fg]
  const n = Number(/^38;5;(\d+)$/.exec(fg)?.[1])
  if (!(n >= 16 && n <= 255)) return undefined
  const hex = (v: number) => v.toString(16).padStart(2, "0")
  if (n >= 232) return `#${hex(8 + (n - 232) * 10).repeat(3)}`
  const level = (v: number) => (v === 0 ? 0 : 55 + v * 40)
  const c = n - 16
  return `#${hex(level(Math.floor(c / 36)))}${hex(level(Math.floor(c / 6) % 6))}${hex(level(c % 6))}`
}

const tui: TuiPlugin = async (api) => {
  const [lines, setLines] = createSignal<Segment[][]>([])

  // Everything the binary needs. The home screen has no session: every session field stays absent.
  const collect = () => {
    const route = api.route.current
    const sessionID = route.name === "session" ? (route.params as { sessionID?: string }).sessionID : undefined
    const all = sessionID ? api.state.session.messages(sessionID) : []
    const assistant = all.filter((m): m is AssistantMessage => m.role === "assistant")
    const last = assistant.at(-1)
    const model = last ? `${last.providerID}/${last.modelID}` : api.state.config.model
    const [providerID, ...rest] = (model ?? "").split("/")
    const bash: { cmd: string; start: number; end: number }[] = []
    for (const m of assistant)
      for (const p of api.state.part(m.id))
        if (p.type === "tool" && p.tool === "bash" && p.state.status === "completed")
          bash.push({ cmd: String(p.state.input.command ?? ""), start: p.state.time.start, end: p.state.time.end })
    return {
      sessionID,
      cwd: api.state.path.directory,
      model,
      variant: last?.variant,
      contextLimit: api.state.provider.find((p) => p.id === providerID)?.models[rest.join("/")]?.limit.context,
      started: all[0]?.time.created,
      messages: assistant.map((m) => ({ tokens: m.tokens, cost: m.cost, completed: m.time.completed })),
      bash,
    }
  }

  let running = false
  const refresh = () => {
    if (running) return
    running = true
    try {
      const payload = JSON.stringify(collect())
      const args = ["--agent", "opencode", "--format", "json"]
      if (api.renderer.width > 1) args.push("--width", String(api.renderer.width - 1))
      const child = execFile(BIN, args, { timeout: LIMIT_MS, windowsHide: true }, (err, out) => {
        running = false
        if (err) return
        try {
          const parsed = JSON.parse(out)
          if (Array.isArray(parsed) && parsed.every(Array.isArray)) setLines(parsed)
        } catch {}
      })
      // A missing binary closes stdin at once; unhandled, that error would take the TUI down.
      child.stdin?.on("error", () => {})
      child.stdin?.end(payload)
    } catch {
      running = false
    }
  }

  refresh()
  const tick = setInterval(refresh, INTERVAL_MS)
  api.lifecycle.onDispose(() => clearInterval(tick))

  api.slots.register({
    slots: {
      app_bottom: () => (
        <box flexDirection="column" paddingLeft={1}>
          <For each={lines()}>
            {(line) => (
              <text>
                <For each={line}>
                  {(s) => (
                    <span style={{ fg: s.dim ? DIM : colour(s.fg) }}>{s.bold ? <b>{s.text}</b> : s.text}</span>
                  )}
                </For>
              </text>
            )}
          </For>
        </box>
      ),
    },
  })
}

export default { id: "rainbow.statusline", tui } satisfies TuiPluginModule & { id: string }
