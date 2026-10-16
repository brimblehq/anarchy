# brimble-cli

Official CLI for [Brimble](https://brimble.io).

## Install

Homebrew (macOS, Linux):

```bash
brew install brimblehq/tap/brimble
```

Install script (macOS, Linux) — downloads the latest release, verifies its
checksum and installs to `/usr/local/bin` (or `~/.local/bin`):

```bash
curl -fsSL https://brimble-cli.t3.tigrisfiles.io/install.sh | sh

# pin a version or choose the directory
curl -fsSL https://brimble-cli.t3.tigrisfiles.io/install.sh | BRIMBLE_VERSION=v0.1.0 BRIMBLE_INSTALL_DIR=~/bin sh
```

Every GitHub release also carries the script, so this works before the S3 URL
exists: `https://github.com/brimblehq/anarchy/releases/latest/download/install.sh`.

Windows: download the zip for your architecture from the
[releases page](https://github.com/brimblehq/anarchy/releases).

Upgrade later with `brimble upgrade` (uses brew or `go install` when that's how
it was installed; otherwise downloads, verifies and replaces itself).
The CLI mentions new versions after a command finishes (at most once a day; off
in CI and with `BRIMBLE_NO_UPDATE_NOTIFIER=1`).

From source:

```bash
make build                     # binary: ./bin/brimble
go install github.com/brimblehq/anarchy/cmd/brimble@latest
```

## Quick start

```bash
brimble login          # paste an API key, pick a default workspace
cd my-app
brimble deploy         # link or create a project, deploy, follow the build
brimble logs           # stream live application logs
```

`brimble deploy` in a directory that isn't linked yet offers to link an
existing project or create a new one (from GitHub, GitLab, Bitbucket, a Docker
image, or by uploading a local folder as a static site). It then follows the
build, printing log lines as they arrive, and exits non-zero if the deployment
fails.

## Coding agents (MCP)

Connect Brimble's MCP server to the coding agents on your machine so you can
ask them to deploy, read logs or manage projects:

```bash
brimble mcp                          # detects your agents; pick, choose sign-in, done
brimble mcp cursor claude-code       # specific agents
brimble mcp --all --auth api-key -y  # every detected agent, no prompts (CI/dotfiles)
brimble mcp status                   # which agents have Brimble
brimble mcp disconnect cursor
```

Supported: Claude Code, Cursor, VS Code (Copilot), Windsurf, Codex, Gemini CLI,
opencode and Zed are configured automatically; Claude Desktop and Grok Build get
step-by-step instructions (they're set up inside the app).

Sign-in: **browser** (default) writes only the server URL and the agent asks
you to approve Brimble on first use — no secret is saved, and you don't need
`brimble login`. **api-key** reuses this CLI's key (checked first), so the
agent works immediately; the key is stored in that agent's config. Zed only
supports the API key.

Only the `brimble` entry is touched: other servers, settings and key order are
kept, and files with comments are never rewritten (you get the snippet to
paste instead).

## Linked directories

`brimble link` writes `.brimble/project.json` (project and team IDs — no
secrets). Commands run in that directory or below it default to the linked
project, so `brimble status`, `brimble env list` and `brimble logs` need no
arguments. `brimble unlink` removes the link.

Without a link or a project argument, commands show a picker in a terminal and
fail with a clear message in scripts.

## Auth and workspaces

```bash
brimble login                      # interactive; offers to open the API keys page
brimble login --api-key br_... --team acme
brimble whoami                     # user, active workspace and where it came from
brimble team switch acme           # change the default workspace ("personal" to go back)
brimble logout                     # removes the key and default team
```

Credentials are saved to your user config dir (`brimble/config.json`).
Order: `--api-key` → `BRIMBLE_API_KEY` → config file.
Team: `--team` → `BRIMBLE_TEAM_ID` → linked project's team → `team switch` default.
`--team` accepts a team ID, name or slug.

## Commands

Run `brimble --help`, or `brimble <command> --help`, for every command and flag. The full guide is in the docs:

- [Overview](https://paper.brimble.io/cli/overview): install, log in, link a directory
- [Deploy](https://paper.brimble.io/cli/deploy)
- [Projects](https://paper.brimble.io/cli/projects)
- [Config as code](https://paper.brimble.io/cli/config)
- [Logs](https://paper.brimble.io/cli/logs)
- [Environment variables](https://paper.brimble.io/cli/environment-variables)
- [Domains and teams](https://paper.brimble.io/cli/domains-and-teams)
- [Coding agents (MCP)](https://paper.brimble.io/cli/agents)
- [CI](https://paper.brimble.io/cli/ci)

## CI

```bash
export BRIMBLE_API_KEY=br_...
brimble deploy my-api              # waits for the build; exit code 1 on failure
brimble deploy my-api --detach -o json   # {"projectId": ..., "logId": ...}
brimble env set my-api RELEASE=$GIT_SHA
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

Watchable commands (`project list|get|stats`, `logs list`) also accept
`-w, --watch` and `--interval` (default `5s`).

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
internal/config/      env + flags + config file, directory links
internal/auth/        credentials
internal/client/      HTTP + Core/Auth API methods
internal/ui/          prompts, spinner, colors
internal/output/      table / json printers
internal/cmd/         cobra commands
```

## License

Proprietary — Brimble Inc.
