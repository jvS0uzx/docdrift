package extract

import "testing"

func TestPathToken(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"src/map/world.ts", "src/map/world.ts", true},
		{"./scripts/auth.js", "scripts/auth.js", true},
		{"internal/check/check.go:42", "internal/check/check.go", true},
		{"src/app/(main)/page.tsx", "src/app/(main)/page.tsx", true},
		{"backend/dist/", "backend/dist/", true},
		{"~/.ssh/id_rsa", "~/.ssh/id_rsa", true},
		{"/home/me/My Notes/a.md", "/home/me/My Notes/a.md", true},
		{"cmd/app", "", false},
		{"go.mod", "", false},
		{"go test ./...", "", false},
		{"actions/checkout@v4", "", false},
		{"https://x.dev/a.js", "", false},
		{"image:tag/x.yml", "", false},
		{"--config path/x.yml", "", false},
		{"application/json", "", false},
		{"a/b c/d.go", "", false},
		{"C:\\Users\\x\\a.go", "", false},
		{"feat/new-thing", "", false},
	}
	for _, c := range cases {
		got, ok := PathToken(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("PathToken(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestParse(t *testing.T) {
	md := "---\n" +
		"repo: https://github.com/x/y — /srv/code/y\n" +
		"---\n" +
		"See `src/a.go` and [guide](docs/guide.md#setup).\n" +
		"```sh\ncat `src/in-fence.go` && git clone https://github.com/me/fenced\n```\n" +
		"The old name was `src/legacy.go`, since renamed.\n" +
		"Skip `src/skip.go` <!-- docdrift:ignore -->\n" +
		"Clone github.com/jvS0uzx/dock_keeper.git and see github.com/orgs/x.\n" +
		"Template `migrations/NNN_name.sql` and `seuusuario/repo/x.md`, see github.com/owner/repo.\n" +
		"Also `notes/[Title].md` and `00_Sistema/.../Infra.md`.\n"

	doc := Parse("n.md", []byte(md))
	if doc.Root != "/srv/code/y" {
		t.Fatalf("Root = %q", doc.Root)
	}
	type key struct {
		kind  Kind
		value string
		line  int
	}
	got := map[key]Claim{}
	for _, c := range doc.Claims {
		got[key{c.Kind, c.Value, c.Line}] = c
	}
	want := []key{
		{KindPath, "src/a.go", 4},
		{KindLink, "docs/guide.md", 4},
		{KindGitHub, "me/fenced", 6},
		{KindPath, "src/legacy.go", 8},
		{KindGitHub, "jvS0uzx/dock_keeper", 10},
		{KindPath, "migrations/NNN_name.sql", 11},
		{KindPath, "seuusuario/repo/x.md", 11},
		{KindGitHub, "owner/repo", 11},
		{KindPath, "notes/[Title].md", 12},
		{KindPath, "00_Sistema/.../Infra.md", 12},
	}
	if len(doc.Claims) != len(want) {
		t.Fatalf("got %d claims: %+v", len(doc.Claims), doc.Claims)
	}
	for _, w := range want {
		c, ok := got[w]
		if !ok {
			t.Fatalf("missing claim %+v in %+v", w, doc.Claims)
		}
		if hist := w.line == 8; c.Historical != hist {
			t.Errorf("%s: Historical = %v", w.value, c.Historical)
		}
		if ph := w.line >= 11; c.Placeholder != ph {
			t.Errorf("%s: Placeholder = %v", w.value, c.Placeholder)
		}
	}
}

func TestParseIgnoreFile(t *testing.T) {
	doc := Parse("n.md", []byte("<!-- docdrift:ignore-file -->\n`src/a.go`\n"))
	if len(doc.Claims) != 0 {
		t.Fatalf("got %+v", doc.Claims)
	}
}

func TestDocumentedDirectivesDoNotApply(t *testing.T) {
	md := "Use `<!-- docdrift:ignore-file -->` to skip a file.\n" +
		"```md\n<!-- docdrift:ignore-file -->\n```\n" +
		"See `src/a.go`, and `<!-- docdrift:ignore -->` for a line.\n"
	if doc := Parse("n.md", []byte(md)); len(doc.Claims) != 1 {
		t.Fatalf("got %+v", doc.Claims)
	}
}

func TestParseEnvExampleIsNotPlaceholder(t *testing.T) {
	doc := Parse("n.md", []byte("Copy `backend/.env.example`.\n"))
	if len(doc.Claims) != 1 || doc.Claims[0].Placeholder {
		t.Fatalf("got %+v", doc.Claims)
	}
}
