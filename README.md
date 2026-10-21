<p align="center">
  <img src="assets/docdrift.png" alt="docdrift" width="320">
</p>

# docdrift

**English** · [Português](README.pt-BR.md)

Find the lies in your docs before your AI agent believes them.

<p align="center">
  <img src="assets/demo.gif" alt="docdrift finding a moved file, a broken link and a renamed repository" width="800">
</p>

READMEs, `CLAUDE.md`, `AGENTS.md` and personal notes describe a tree that keeps moving. A file gets
renamed, a repository changes owner, a folder ends up inside a monorepo, and the doc still says the
old thing. A person gets suspicious; a coding agent reads the doc as truth and acts on it.

`docdrift` pulls the checkable claims out of your markdown and verifies each one against the disk
and, if you ask, against GitHub:

- paths in inline code: `` `internal/api/server.go` ``, `` `scripts/` ``
- relative links: `[guide](docs/guide.md)`
- GitHub repositories: `github.com/owner/repo`, including inside code blocks
- home paths (`~/.config/x`, `/home/you/...`), when you ask for them

When a path is gone, it looks for a file with the same name elsewhere in the repository and
suggests where it went:

```
$ docdrift -github .
AGENTS.md:4: path does not exist: internal/db/postgres.go
    maybe moved to: api/internal/store/postgres.go
AGENTS.md:6: link target does not exist: docs/rollback.md
AGENTS.md:7: GitHub repository was renamed to moby/moby: docker/docker
2 files, 8 claims, 3 findings (skipped: 0 inactive note, 1 historical, 0 negated, 0 placeholder, 0 generated, 1 unanchored, 0 out of scope)
```

Single static binary, no config file, no dependencies outside the Go standard library.

## Install

```sh
go install github.com/jvS0uzx/docdrift/cmd/docdrift@latest
```

Or grab a prebuilt binary for Linux, macOS or Windows from the
[releases page](https://github.com/jvS0uzx/docdrift/releases).

## Usage

```sh
docdrift                       # every .md under the current directory
docdrift README.md docs/       # specific files and directories
docdrift -github .             # also check repositories (GITHUB_TOKEN or GH_TOKEN)
docdrift -format json . > drift.json
```

Exit code is `1` when there are findings, `2` on usage errors and `0` otherwise (or always `0` with
`-exit-zero`), so it drops into CI as is.

| Flag | Effect |
|---|---|
| `-github` | Looks up every `github.com/owner/repo` reference. Reports deleted, private, renamed or transferred repositories. |
| `-home` | Checks `~/` and absolute paths inside your home. Meant for personal notes about the machine running the check. |
| `-also DIR` | Extra directory relative paths may resolve against, for docs that describe a sibling repository. Repeatable. |
| `-exclude GLOB` | Skips files and directories by name, e.g. `-exclude Releases -exclude 'CHANGELOG*'`. Repeatable. |
| `-strict` | Also reports unanchored paths (see below). |
| `-history` | Also checks lines that talk about the past or about absence, and inactive notes. |
| `-format json` | Machine-readable findings and stats. |

## What it doesn't flag, and why

Detecting drift is easy; the hard part is staying quiet when the doc is right. These cases are
skipped by default and counted in the summary line:

- **Inactive note.** Frontmatter with `status:` deprecated, archived, inactive or obsolete (or the
  Portuguese equivalents), or those words in `tags:`. The whole note is a record of the past.
- **Historical line.** A line that says *renamed*, *removed*, *formerly*, *legacy*, *used to*,
  *no longer* describes the past on purpose.
- **Negated line.** A line that already says the thing doesn't exist: *does not exist*,
  *missing*, *gone*, *404*, ❌. The doc is right.
- **Placeholder.** `<name>`, `{id}`, `[Title]`, `NNN_name.sql`, `YYYYMMDD`, `your-username/`,
  `owner/repo`, `docs/.../x.md`.
- **Generated path.** Anything the root `.gitignore` ignores, plus `node_modules`, `dist`, `build`,
  `target`, `.venv` and friends.
- **Unanchored.** If `foo/bar.go` doesn't exist *and* there is no `foo/` anywhere in the root, the
  doc probably describes another tree, and guessing which one would be noise. `src/gone.go` in a
  repository that has `src/` is reported. Use `-strict` to see these too.
- **Out of scope.** Absolute paths outside your home (`/etc/nginx/...` usually describes a server),
  home paths without `-home`, and GitHub references without `-github`.
- **Docs-site routes.** A root-relative link with no extension (`/api/authentication`) is a page
  on a docs site, not a file. Extensionless relative links (`../guide/setup`) resolve to
  `setup.md`, `setup.mdx` or `setup/index.md`, the way Docusaurus and MkDocs do, and `/images/x.png`
  may resolve from any folder between the file and the root.
- **Not a repository.** `docs.github.com/...`, `github.com/user-attachments/...`, badge URLs that
  end in `.svg`, and Go identifiers like `github.com/google/uuid.UUID` are read for what they are.

Past-tense and negation words are matched against the prose of the line only, not against the
paths: a file named `legacy.go` or `gone.go` doesn't turn the check off. Both English and
Portuguese wording is recognized.

Paths are tried relative to the markdown file, to the repository root and to every top-level
folder of the root. So a doc that says `internal/db.go` <!-- docdrift:ignore -->
in a monorepo whose code lives in `backend/internal/db.go` doesn't raise a false alarm.

### Notes that live outside the code

A note in an Obsidian vault or a wiki can say which checkout it describes with a `repo:` key in its
frontmatter. Relative paths in that note resolve against it:

```yaml
---
repo: /home/you/code/my-service
---
```

### Silencing a line or a file

```md
The deploy key lives at `~/.ssh/deploy_key` on the server. <!-- docdrift:ignore -->
```

`<!-- docdrift:ignore-file -->` anywhere in a file skips the whole file. Directives only count as
an HTML comment in prose: quoted inside code, like here, they do nothing.

## CI

As a GitHub Action:

```yaml
- uses: actions/checkout@v4
- uses: jvS0uzx/docdrift@v0.1.2
  with:
    args: -github .
```

The action passes the workflow's `GITHUB_TOKEN` to `-github` for you. Or install the binary
yourself:

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

- A relative path only counts if it has a known extension or a trailing slash. `cmd/app` could be a
  branch, a MIME type or `owner/repo`, so it isn't checked.
- Only the root `.gitignore` is read; `**` in the middle of a pattern matches a single segment.
- Past-tense and negation words apply to the whole line: a line that negates one thing and mentions
  another, live one, leaves the second unchecked.
- Function names, ports, environment variables and commands quoted in docs aren't checked yet.

## License

[MIT](LICENSE)
