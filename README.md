# brimble-cli

Official CLI for [Brimble](https://brimble.io).

Authenticate with a **Brimble API key**. Every Core / Auth request sends:

```http
x-brimble-key: br_...
```

Optional team context on Core:

```http
x-brimble-team: <teamObjectId>
```

## Install

From a [GitHub Release](https://github.com/brimbleapp/brimble-cli/releases) (recommended):

```bash
# macOS / Linux — pick the archive for your OS/arch from the release assets
tar -xzf brimble_Darwin_arm64.tar.gz   # example
sudo mv brimble /usr/local/bin/
brimble --version
```

From source:

```bash
make build
# binary: ./bin/brimble

# or
go install github.com/brimbleapp/brimble-cli/cmd/brimble@latest
```

## Auth

Interactive (recommended):

```bash
brimble auth login
# → offers to open https://app.brimble.io/settings/api-keys
# → paste key (hidden input)
# → validates with Auth GET /user/me
# → saves to ~/.config/brimble/config.json

brimble auth whoami
brimble auth logout
```

Non-interactive / CI:

```bash
export BRIMBLE_API_KEY=br_...
# or
brimble auth login --api-key br_...
```

Credential order: `--api-key` → `BRIMBLE_API_KEY` → config file.

Missing project/team args prompt an interactive picker when a TTY is present.
Destructive actions (`delete`, `logout`) confirm first; use `-y` to skip.

## Usage

```bash
brimble meta regions

brimble project list
brimble project get my-app
brimble project redeploy <projectId>
brimble project cancel <projectId> <logId>
brimble project delete <projectId>
brimble project stats <projectId>
brimble project stats <projectId> -w
brimble project create -f body.json

brimble env list <projectId>
brimble env set <projectId> --name API_URL --value https://example.com
brimble env delete <projectId> <envId>

brimble log list
brimble log search -q failed
brimble log download
brimble log requests
brimble log stream              # live app logs (Socket.IO /loki tail)
brimble log stream my-app -q error --since 15m

brimble team list
brimble team get acme
brimble team members acme

# machine output
brimble -o json project list
```

## Global flags

| Flag | Env | Default |
|------|-----|---------|
| `--api-key` | `BRIMBLE_API_KEY` | — |
| `--team` | `BRIMBLE_TEAM_ID` | — |
| `--base-url` | `BRIMBLE_CORE_BASE_URL` | `https://api.brimble.io/core` |
| `--auth-url` | `BRIMBLE_AUTH_BASE_URL` | `https://api.brimble.io/auth` |
| `-o, --output` | — | `table` (`json` supported) |
| `--timeout` | — | `30s` |

Watchable commands (`project list|get|stats`, `log list`) also accept:

| Flag | Default |
|------|---------|
| `-w, --watch` | off — refresh the view until Ctrl+C |
| `--interval` | `5s` — refresh cadence when watching |

## UX

Interactive UI is built on:

| Library | Role |
|---------|------|
| [cobra](https://github.com/spf13/cobra) | Commands / flags |
| [huh](https://github.com/charmbracelet/huh) | Forms, password input, select, confirm |
| [lipgloss](https://github.com/charmbracelet/lipgloss) | Colors / styled output |
| [spinner](https://github.com/briandowns/spinner) | Loading indicators |

TTY auto-detect: spinners and forms only run in a real terminal. CI uses flags/env.

## Layout

```
cmd/brimble/          entrypoint
internal/config/      env + flags + config file
internal/auth/        credentials
internal/client/      HTTP + Core/Auth API methods
internal/ui/          prompts, spinner, colors
internal/output/      table / json printers
internal/cmd/         cobra commands
```

## Release

CI runs on every push/PR (vet, test, build, `goreleaser check`).

To ship a new version, tag and push:

```bash
git tag v0.1.0
git push origin v0.1.0
```

That triggers [.github/workflows/release.yml](.github/workflows/release.yml), which runs [GoReleaser](https://goreleaser.com) and publishes archives + checksums to the GitHub Release.

Local dry-runs (GoReleaser is pulled via `go run` if you don’t have it installed):

```bash
make snapshot        # build dist/ without publishing (works without git)
make release-dry-run # full pipeline without publish (needs a git repo + tag; falls back to snapshot otherwise)
```

## License

Proprietary — Brimble Inc.
