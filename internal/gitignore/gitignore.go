package gitignore

import (
	"bufio"
	"os"
	"path"
	"strings"
)

type rule struct {
	pattern  string
	negate   bool
	dirOnly  bool
	anchored bool
}

type Matcher struct {
	rules []rule
}

func Load(file string) *Matcher {
	m := &Matcher{}
	f, err := os.Open(file)
	if err != nil {
		return m
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m.Add(sc.Text())
	}
	return m
}

func (m *Matcher) Add(line string) {
	line = strings.TrimRight(line, " \t\r")
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}
	var r rule
	if strings.HasPrefix(line, "!") {
		r.negate = true
		line = line[1:]
	}
	if strings.HasSuffix(line, "/") {
		r.dirOnly = true
		line = strings.TrimRight(line, "/")
	}
	if strings.HasPrefix(line, "**/") {
		line = strings.TrimPrefix(line, "**/")
	} else if strings.Contains(line, "/") {
		r.anchored = true
	}
	r.pattern = strings.TrimPrefix(line, "/")
	r.pattern = strings.ReplaceAll(r.pattern, "/**/", "/*/")
	if r.pattern != "" {
		m.rules = append(m.rules, r)
	}
}

func (m *Matcher) Match(rel string, isDir bool) bool {
	rel = strings.Trim(path.Clean("/"+rel), "/")
	if rel == "" || len(m.rules) == 0 {
		return false
	}
	parts := strings.Split(rel, "/")
	for i := range parts {
		p := strings.Join(parts[:i+1], "/")
		last := i == len(parts)-1
		if m.matchOne(p, !last || isDir) {
			return true
		}
	}
	return false
}

func (m *Matcher) matchOne(p string, isDir bool) bool {
	ignored := false
	for _, r := range m.rules {
		if r.dirOnly && !isDir {
			continue
		}
		target := p
		if !r.anchored {
			target = path.Base(p)
		}
		if ok, _ := path.Match(r.pattern, target); ok {
			ignored = !r.negate
		}
	}
	return ignored
}
