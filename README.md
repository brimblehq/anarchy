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

```bash
# Deploy & observe
brimble deploy [project]           # --detach, --no-logs, --restart, --dir ./dist (static uploads)
brimble status [project]           # details + recent deployments
brimble open [project]             # open the live URL
brimble logs [project]             # live logs; -q error --since 1h
brimble logs list                  # deployment history (--status FAILED, --watch)
brimble logs download [project] [deploymentId]
brimble logs requests              # HTTP requests; --status 500 --method POST
brimble logs search -q "fix login"

# Projects
brimble project list               # --type, --status, --environment, --watch
brimble project get [project]
brimble project create             # interactive, or -f body.json
brimble project redeploy | rollback | restart | cancel [project]
brimble project start | stop [project]
brimble project maintenance [project] on|off
brimble project password [project] on|off
brimble project previews [project] on|off
brimble project transfer [project] --to <team>
brimble project connect-repo | disconnect-repo [project]
brimble project stats [project]    # CPU, memory, traffic, latency
brimble project delete [project]   # asks you to type the name; --delete-volume

# Environment variables
brimble env list [--reveal]
brimble env set API_URL=https://... DEBUG=false
brimble env set STRIPE_KEY         # bare KEY prompts with hidden input
brimble env delete OLD_TOKEN
brimble env pull -f .env           # decrypt into a file (0600)
brimble env push -f .env.production
# -e preview targets preview deployments

# Domains, environments, teams
brimble domain list | add <domain> | remove <domain> | status <domain>
brimble environment list | create <name> | delete <name> --move-to <name> | default <name>
brimble team list | members | invite <email>... | remove <email>

brimble meta regions | plans | frameworks
```

Add `-o json` to any command for machine-readable output; status messages go
to stderr so stdout stays parseable. Destructive actions confirm first; pass
`-y` to skip (required in non-interactive shells).

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
