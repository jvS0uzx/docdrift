package check

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func reasons(fs []Finding) map[string]Finding {
	out := map[string]Finding{}
	for _, f := range fs {
		out[f.Value] = f
	}
	return out
}

func TestFile(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	sibling := t.TempDir()
	write(t, filepath.Join(repo, ".git", "HEAD"), "ref: refs/heads/main\n")
	write(t, filepath.Join(repo, ".gitignore"), "/data/\n")
	write(t, filepath.Join(repo, "src", "ok.go"), "")
	write(t, filepath.Join(repo, "game", "java", "src", "shared", "x.go"), "")
	write(t, filepath.Join(repo, "backend", "internal", "db.go"), "")
	write(t, filepath.Join(repo, "docs", "guide.md"), "")
	write(t, filepath.Join(home, ".config", "app.toml"), "")
	write(t, filepath.Join(sibling, "apps", "api", "main.go"), "")

	readme := filepath.Join(repo, "README.md")
	md := strings.Join([]string{
		"`src/ok.go` exists",
		"`src/gone.go` does not",
		"`src/shared/` moved",
		"[guide](docs/guide.md) and [broken](docs/missing.md)",
		"`data/raw.csv` is gitignored, `frontend/dist/` is build output",
		"`~/.config/app.toml` exists, `~/.config/nope.toml` does not",
		"`/etc/nginx/nginx.conf` describes a server",
		"`apps/api/main.go` lives in the sibling repo",
		"`src/legacy.go` was renamed",
		"`migrations/NNN_name.sql` is a template",
		"`internal/db.go` lives under backend/ in this monorepo",
		"`Releases/2026/note.md` belongs to some other tree",
		"`src/hooks/post.sh` ❌ não existe mais",
	}, "\n")
	write(t, readme, md)

	c := New(Options{Home: home, CheckHome: true, Also: []string{sibling}})
	got := reasons(c.File(readme, []byte(md)))

	for _, v := range []string{"src/gone.go", "src/shared/", "docs/missing.md", "~/.config/nope.toml"} {
		if _, ok := got[v]; !ok {
			t.Errorf("expected finding for %s; got %+v", v, got)
		}
	}
	if len(got) != 4 {
		t.Errorf("expected 4 findings, got %d: %+v", len(got), got)
	}
	if s := got["src/shared/"].Suggestions; len(s) != 1 || s[0] != "game/java/src/shared" {
		t.Errorf("suggestions = %v", s)
	}
	if got["docs/missing.md"].Reason != "destino do link não existe" {
		t.Errorf("reason = %q", got["docs/missing.md"].Reason)
	}
	want := Stats{Files: 1, Claims: 16, Findings: 4, Historical: 1, Negated: 1, Placeholder: 1, Ignored: 2, Unresolved: 1, OutOfScope: 1}
	if c.Stats != want {
		t.Errorf("stats = %+v, want %+v", c.Stats, want)
	}
}

func TestHomePathsNeedOptIn(t *testing.T) {
	c := New(Options{Home: t.TempDir()})
	got := c.File(filepath.Join(t.TempDir(), "a.md"), []byte("`~/.my.cnf`\n"))
	if len(got) != 0 || c.Stats.OutOfScope != 1 {
		t.Fatalf("got %+v, stats %+v", got, c.Stats)
	}
}

func TestInactiveNoteIsSkipped(t *testing.T) {
	repo := t.TempDir()
	for _, fm := range []string{"status: DESATIVADA", "tags: [automacao, historico]", "tags:\n  - projeto\n  - arquivado"} {
		md := "---\n" + fm + "\n---\n`src/gone.go`\n"
		c := New(Options{Home: t.TempDir()})
		if got := c.File(filepath.Join(repo, "a.md"), []byte(md)); len(got) != 0 || c.Stats.Inactive != 1 {
			t.Errorf("%q: got %+v, stats %+v", fm, got, c.Stats)
		}
		c = New(Options{Home: t.TempDir(), History: true})
		write(t, filepath.Join(repo, "src", "keep.go"), "")
		if got := c.File(filepath.Join(repo, "a.md"), []byte(md)); len(got) != 1 {
			t.Errorf("%q with History: got %+v", fm, got)
		}
	}
}

func TestActiveStatusIsChecked(t *testing.T) {
	repo := t.TempDir()
	write(t, filepath.Join(repo, "src", "keep.go"), "")
	md := "---\nstatus: ativo\n---\n`src/gone.go`\n"
	if got := New(Options{Home: t.TempDir()}).File(filepath.Join(repo, "a.md"), []byte(md)); len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestStrictReportsUnresolved(t *testing.T) {
	repo := t.TempDir()
	md := "`Releases/2026/note.md`\n"
	got := New(Options{Home: t.TempDir(), Strict: true}).File(filepath.Join(repo, "a.md"), []byte(md))
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestFrontmatterRoot(t *testing.T) {
	code := t.TempDir()
	vault := t.TempDir()
	write(t, filepath.Join(code, "cmd", "main.go"), "")
	note := filepath.Join(vault, "Project.md")
	md := "---\nrepo: " + code + "\n---\n`cmd/main.go` and `cmd/old.go`\n"

	got := reasons(New(Options{Home: t.TempDir()}).File(note, []byte(md)))
	if len(got) != 1 || got["cmd/old.go"].Root != code {
		t.Fatalf("got %+v", got)
	}
}

func TestGitHub(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/repos/me/alive":
			w.Write([]byte(`{"full_name":"me/alive"}`))
		case "/repos/me/old":
			w.Write([]byte(`{"full_name":"me/new"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	md := "github.com/me/alive github.com/me/old github.com/me/gone\nagain github.com/me/old\n"
	c := New(Options{GitHub: true, GitHubAPI: srv.URL, Home: t.TempDir()})
	fs := c.File(filepath.Join(t.TempDir(), "a.md"), []byte(md))
	if len(fs) != 3 {
		t.Fatalf("got %+v", fs)
	}
	if !strings.Contains(fs[0].Reason, "renomeado para me/new") || !strings.Contains(fs[1].Reason, "não encontrado") {
		t.Errorf("reasons: %q, %q", fs[0].Reason, fs[1].Reason)
	}
	if calls != 3 {
		t.Errorf("expected cached lookups, got %d calls", calls)
	}
}

func TestGitHubRateLimitStopsChecks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := New(Options{GitHub: true, GitHubAPI: srv.URL, Home: t.TempDir()})
	fs := c.File(filepath.Join(t.TempDir(), "a.md"), []byte("github.com/a/b github.com/c/d\n"))
	if len(fs) != 0 || c.GitHubUnavailable() == "" {
		t.Fatalf("findings %+v, unavailable %q", fs, c.GitHubUnavailable())
	}
}
