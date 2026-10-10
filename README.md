# docdrift

Find the lies in your docs before your AI agent believes them.

READMEs, `CLAUDE.md`, `AGENTS.md` and personal notes describe a tree that keeps
moving. A file gets renamed, a repository changes owner, a directory is folded
into a monorepo, and the doc keeps saying the old thing. A human shrugs; a
coding agent reads the doc as ground truth and acts on it.

`docdrift` pulls checkable claims out of markdown and verifies them against the
filesystem and, optionally, GitHub:

- paths in inline code: `` `internal/api/server.go` ``, `` `scripts/` ``
- relative markdown links: `[guide](docs/guide.md)`
- GitHub repositories: `github.com/owner/repo`, including inside code blocks
- home paths (`~/.config/x`, `/home/you/...`), when you ask for them

When a path is gone, it looks for a file with the same name elsewhere in the
repository and suggests where it went:

```
GAME_2D.md:25: path does not exist: src/shared/
    maybe moved to: game/java/src/shared
README.md:22: github repository was renamed to jvS0uzx/gourmetiz-app: JoaoVbsz/gourmetiz-app
```

## Install

```sh
go install github.com/jvS0uzx/docdrift/cmd/docdrift@latest
```

## Use

```sh
docdrift                       # every .md under the current directory
docdrift README.md docs/       # specific files and directories
docdrift -github .             # also check GitHub repositories (GITHUB_TOKEN or GH_TOKEN)
docdrift -format json . > drift.json
```

Exit status is `1` when there are findings, `2` on usage errors, `0` otherwise
(or always `0` with `-exit-zero`), so it drops into CI as is.

| Flag | Effect |
|---|---|
| `-github` | Look up every `github.com/owner/repo` reference. Reports repositories that are gone, private, renamed or transferred. |
| `-home` | Check `~/` paths and absolute paths under your home directory. Meant for personal notes about the machine running the check. |
| `-also DIR` | Extra directory relative paths may resolve against, for docs that describe a sibling repository. Repeatable. |
| `-exclude GLOB` | Skip files and directories by name, e.g. `-exclude Releases -exclude 'CHANGELOG*'`. Repeatable. |
| `-strict` | Report paths whose first segment exists nowhere in the root (see below). |
| `-history` | Also check lines that talk about the past. |
| `-format json` | Machine-readable findings and stats. |

## What it does not report, and why

Drift detection is easy; staying quiet when a doc is right is the hard part.
These are skipped by default and counted in the summary line:

- **Historical lines.** A line that says *renamed*, *removed*, *formerly*,
  *legacy* (or the Portuguese *antigo*, *renomeado*, *era*...) describes the
  past on purpose.
- **Placeholders.** `<name>`, `{id}`, `[Title]`, `NNN_name.sql`,
  `AAAAMMDD`, `your-user/`, `seu-usuario/`, `00_Sistema/.../x.md`.
- **Generated paths.** Anything matched by the root `.gitignore`, plus
  `node_modules`, `dist`, `build`, `target`, `.venv` and similar.
- **Unresolved paths.** If `foo/bar.go` is missing *and* there is no `foo/`
  anywhere in the root, the doc is most likely describing another tree. Guessing
  which would be noise. `src/gone.go` in a repository that has `src/` is
  reported. Use `-strict` to see the unresolved ones too.
- **Out of scope.** Absolute paths outside your home (`/etc/nginx/...` usually
  describes a server), home paths without `-home`, GitHub references without
  `-github`.

Paths are tried relative to the markdown file, to the repository root and to
each top-level directory of the root, so a doc that says `internal/db.go` <!-- docdrift:ignore -->
in a monorepo whose code lives in `backend/internal/db.go` is not a false alarm.

### Notes that live outside the code

A note in an Obsidian vault or a wiki can say which checkout it describes with
a `repo:` frontmatter key. Relative paths in that note resolve against it:

```yaml
---
repo: /home/you/code/my-service
---
```

### Silencing a line or a file

```md
The deploy key lives in `~/.ssh/deploy_key` on the server. <!-- docdrift:ignore -->
```

`<!-- docdrift:ignore-file -->` anywhere in a file skips the whole file.

## CI

```yaml
- uses: actions/setup-go@v5
  with:
    go-version: stable
- run: go install github.com/jvS0uzx/docdrift/cmd/docdrift@latest
- run: docdrift -github .
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

## Limitations

- Relative tokens need a known extension or a trailing slash to count as a
  path. `cmd/app` could be a branch, a MIME type or `owner/repo`, so it is
  not checked.
- Only the root `.gitignore` is read; `**` in the middle of a pattern matches a
  single segment.
- Function names, ports, environment variables and commands in docs are not
  checked yet.
