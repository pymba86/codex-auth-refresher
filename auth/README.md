# Auth directory

Place runtime auth JSON files in this directory, for example files created via:

```bash
./cliproxyapi --codex-login
```

Tracked filename prefixes:
- `codex-*.json`
- `gemini-*.json`
- `antigravity-*.json`

Important:
- do not commit `auth/*.json`
- do not commit `auth/*.bak-*`
- copy these files to the target server outside git
- if tokens were exposed in chats, logs, or screenshots, rotate them before production use
