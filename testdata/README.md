# testdata

Real status line input and session data captured from the assistants, scrubbed before commit. Findings per assistant: `docs/discovery.md`.

| File | Source | Content |
|---|---|---|
| `claude/session-start.json` | Claude Code 2.1.316, first status line call | nulls where nothing has run yet |
| `claude/session-after-reply.json` | same session, after one reply | tokens, cost, cache timer, rate limits |
| `copilot/session-start.json` | Copilot CLI 1.0.105, first call | |
| `copilot/session-after-reply.json` | same session, after one reply | `used_percentage` stays null, `current_context_used_percentage` is set |
| `opencode/export-session.json` | `opencode export <id>` (OpenCode 1.19.5, `opencode/big-pickle`) | one user and one assistant message |

## Scrubbing

Every string is replaced by `"redacted"` except values at an allowlisted key path (model ids and names, plugin ids, status enums) and ISO timestamps; an allowlisted value that contains a path, an e-mail, the user name or the host name is redacted anyway. Keys, numbers, booleans and nulls are unchanged. Numeric precision is kept as written.

```
go run scripts/scrub-sample.go raw.json > testdata/<assistant>/<name>.json
go run scripts/scrub-sample.go raw.json testdata/<assistant>/<name>.json
```

The allowlist is the `allowlist` variable in `scripts/scrub-sample.go`. Add a key path there when a new enum should survive.

## Before committing a sample

Never commit raw captures (`collected/` is git-ignored). Check the scrubbed file for user name, host name, e-mail addresses and home paths:

```
grep -rniE "$(id -un)|$(hostname -s)|@|/Users/|/home/|C:\\\\Users" testdata
```

Nothing may match. The repo becomes public.
