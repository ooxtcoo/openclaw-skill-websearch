# websearch

Standalone Go CLI for:
- web search
- URL fetch
- file download
- image download
- AI-friendly JSON output

## Goals

- no Ollama dependency
- no LM Studio dependency
- no Python runtime dependency
- fast local execution
- cross-platform: Windows, Linux, macOS
- output shaped for agent tool use

## Current commands

### Search

```powershell
cd C:\Users\1111\.openclaw\workspace\skills\websearch\go-websearch
.\websearch.exe search "best lightweight markdown editors for windows" --engine auto --max 5
```

### Fetch

```powershell
.\websearch.exe fetch https://github.com/openclaw/openclaw
```

### Download

```powershell
.\websearch.exe download https://example.com/file.zip --save downloads\file.zip
```

### List downloads

```powershell
.\websearch.exe list
```

## Output design

Default output is JSON so local LLMs and agents can parse it reliably.

### Search output

```json
{
  "ok": true,
  "command": "search",
  "query": "OpenClaw latest release",
  "engine": "auto",
  "tried_engines": ["google", "bing", "duckduckgo"],
  "count": 5,
  "results": [
    {
      "engine": "google",
      "title": "...",
      "url": "https://...",
      "display_url": "example.com",
      "rank": 1
    }
  ]
}
```

### Fetch output

Includes:
- final URL after redirects
- HTTP status
- content type
- selected headers
- text preview for HTML/text
- base64 preview for binary/image content
- sha256 checksum

## Build on Windows

```powershell
cd C:\Users\1111\.openclaw\workspace\skills\websearch\go-websearch
go build -trimpath -ldflags "-s -w" -o websearch.exe .\main.go
```

## Build on Linux/macOS

```bash
./build.sh
```

## Publish notes

This package is intended for ClawHub as a **skill**, not a plugin.

## Notes

- `--engine auto` tries Google, then Bing, then DuckDuckGo.
- Some search engines may occasionally block scraping or return sparse HTML. In that case retry with a specific engine.
- `fetch` is best for pages and known URLs.
- `download` is best when the file must be written to disk.
