package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jvS0uzx/docdrift/internal/check"
)

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("docdrift", flag.ContinueOnError)
	fl.SetOutput(stderr)
	format := fl.String("format", "text", "output format: text or json")
	history := fl.Bool("history", false, "also check lines that describe the past (renamed, removed, antigo...)")
	gh := fl.Bool("github", false, "check github.com/owner/repo references against the GitHub API (uses GITHUB_TOKEN or GH_TOKEN)")
	home := fl.Bool("home", false, "check ~/ and absolute paths under the home directory (for personal notes about this machine)")
	strict := fl.Bool("strict", false, "report paths whose first segment exists nowhere in the root (usually references to another tree)")
	exitZero := fl.Bool("exit-zero", false, "exit 0 even when findings exist")
	showVersion := fl.Bool("version", false, "print version")
	var also, exclude multi
	fl.Var(&also, "also", "extra directory relative paths may resolve against (repeatable)")
	fl.Var(&exclude, "exclude", "skip files or directories whose name matches this glob, e.g. Releases or 'CHANGELOG*' (repeatable)")
	fl.Usage = func() {
		fmt.Fprintf(stderr, "usage: docdrift [flags] [path ...]\n\nScans markdown files (default: current directory) and reports paths,\nlinks and GitHub repositories that no longer exist.\n\nflags:\n")
		fl.PrintDefaults()
	}
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, version)
		return 0
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "docdrift: unknown format %q\n", *format)
		return 2
	}

	targets := fl.Args()
	if len(targets) == 0 {
		targets = []string{"."}
	}
	files, err := collect(targets, exclude)
	if err != nil {
		fmt.Fprintln(stderr, "docdrift:", err)
		return 2
	}

	for i, a := range also {
		if abs, err := filepath.Abs(a); err == nil {
			also[i] = abs
		}
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	c := check.New(check.Options{Also: also, History: *history, Strict: *strict, CheckHome: *home, GitHub: *gh, GitHubToken: token})

	var findings []check.Finding
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintln(stderr, "docdrift:", err)
			continue
		}
		findings = append(findings, c.File(f, content)...)
	}

	cwd, _ := os.Getwd()
	for i := range findings {
		findings[i].File = display(cwd, findings[i].File)
	}

	if *format == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(struct {
			Findings []check.Finding `json:"findings"`
			Stats    check.Stats     `json:"stats"`
		}{nonNil(findings), c.Stats})
	} else {
		for _, f := range findings {
			fmt.Fprintf(stdout, "%s:%d: %s: %s\n", f.File, f.Line, f.Reason, f.Value)
			for _, s := range f.Suggestions {
				fmt.Fprintf(stdout, "    maybe moved to: %s\n", s)
			}
		}
		s := c.Stats
		fmt.Fprintf(stderr, "%d files, %d claims, %d findings (skipped: %d historical, %d placeholder, %d ignored, %d unresolved, %d out of scope)\n",
			s.Files, s.Claims, s.Findings, s.Historical, s.Placeholder, s.Ignored, s.Unresolved, s.OutOfScope)
	}
	if why := c.GitHubUnavailable(); why != "" {
		fmt.Fprintln(stderr, "docdrift: GitHub checks stopped:", why)
	}

	if len(findings) > 0 && !*exitZero {
		return 1
	}
	return 0
}

func collect(targets, exclude []string) ([]string, error) {
	skip := func(name string) bool {
		for _, g := range exclude {
			if ok, _ := filepath.Match(g, name); ok {
				return true
			}
		}
		return false
	}
	var files []string
	for _, t := range targets {
		abs, err := filepath.Abs(t)
		if err != nil {
			return nil, err
		}
		st, err := os.Stat(abs)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			files = append(files, abs)
			continue
		}
		err = filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() && p != abs && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" || d.Name() == "vendor" || skip(d.Name())) {
				return fs.SkipDir
			}
			if !d.IsDir() && skip(d.Name()) {
				return nil
			}
			if !d.IsDir() && (strings.HasSuffix(p, ".md") || strings.HasSuffix(p, ".mdx")) {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func display(cwd, p string) string {
	if rel, err := filepath.Rel(cwd, p); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return p
}

func nonNil(f []check.Finding) []check.Finding {
	if f == nil {
		return []check.Finding{}
	}
	return f
}
