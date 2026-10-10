package gitignore

import "testing"

func TestMatch(t *testing.T) {
	m := &Matcher{}
	for _, l := range []string{
		"# comment",
		"*.log",
		"/data/",
		"logs/",
		"build/out",
		"**/tmp",
		"secret*",
		"!secret.example",
	} {
		m.Add(l)
	}
	cases := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"app.log", false, true},
		{"deep/dir/app.log", false, true},
		{"data", true, true},
		{"data/x.csv", false, true},
		{"sub/data/x.csv", false, false},
		{"logs", false, false},
		{"logs/a.txt", false, true},
		{"a/logs/a.txt", false, true},
		{"build/out", true, true},
		{"x/build/out", true, false},
		{"a/b/tmp/c.go", false, true},
		{"secret.env", false, true},
		{"secret.example", false, false},
		{"src/main.go", false, false},
	}
	for _, c := range cases {
		if got := m.Match(c.path, c.isDir); got != c.want {
			t.Errorf("Match(%q, %v) = %v, want %v", c.path, c.isDir, got, c.want)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	if Load("/nonexistent/.gitignore").Match("a.go", false) {
		t.Fatal("empty matcher ignored a path")
	}
}
