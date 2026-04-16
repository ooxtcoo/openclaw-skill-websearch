---
name: websearch
description: Standalone cross-platform web search, fetch, and file download skill backed by a native Go CLI. Use when current web information is needed, when built-in search/fetch tools are unavailable or unsuitable, when running under LM Studio instead of Ollama, or when the task needs a deterministic local CLI for websearch, page fetches, file downloads, or image downloads on Windows, Linux, or macOS.
---

# Websearch Skill

Use the native Go CLI in `go-websearch/`.

## Primary workflow

1. Build the CLI if `websearch.exe` (Windows) or `websearch` (Linux/macOS) is missing.
2. Prefer `search` for current web discovery.
3. Use `fetch` for page/content retrieval when the URL is already known.
4. Use `download` when the file must be saved locally.
5. Prefer JSON output because it is easier for agent/tool chaining.

## Commands

### Search

Windows:
```powershell
.\websearch.exe search "OpenClaw latest release" --engine auto --max 5
```

Linux/macOS:
```bash
./websearch search "OpenClaw latest release" --engine auto --max 5
```

Notes:
- `--engine auto` tries Google, then Bing, then DuckDuckGo.
- Output is JSON by default.
- Good for local LLM tool use because the payload is compact and structured.

### Fetch

```powershell
.\websearch.exe fetch https://github.com/openclaw/openclaw
```

Returns JSON with:
- status
- content type
- final URL after redirects
- headers subset
- text preview for text/HTML
- base64 preview for binary content
- sha256 checksum

### Download

```powershell
.\websearch.exe download https://example.com/file.zip --save downloads\file.zip
```

Use this when the file must exist on disk.

## Build

Windows:
```powershell
cd C:\Users\1111\.openclaw\workspace\skills\websearch\go-websearch
go build -trimpath -ldflags "-s -w" -o websearch.exe .\main.go
```

Linux/macOS:
```bash
go build -trimpath -ldflags "-s -w" -o websearch ./main.go
```

## Files

- `go-websearch/main.go` - standalone CLI implementation
- `README.md` - user-facing usage notes and examples

## Response expectations

Expect JSON output. Parse it instead of scraping console prose.

Important fields:
- Search: `results[]`, `engine`, `tried_engines`, `count`
- Fetch/download: `status`, `content_type`, `preview_text`, `preview_base64`, `saved_to`, `sha256`

## If search engines block

If Google or Bing returns sparse results or blocks the request:
1. Retry with `--engine duckduckgo`
2. Retry later with a narrower query
3. Use `fetch` directly on a known URL when available
