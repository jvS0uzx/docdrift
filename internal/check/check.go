package check

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jvS0uzx/docdrift/internal/extract"
	"github.com/jvS0uzx/docdrift/internal/gitignore"
)

type Finding struct {
	extract.Claim
	Reason      string   `json:"motivo"`
	Root        string   `json:"raiz,omitempty"`
	Suggestions []string `json:"sugestoes,omitempty"`
}

type Options struct {
	Also    []string
	History bool

	Strict      bool
	GitHub      bool
	GitHubToken string
	GitHubAPI   string

	CheckHome bool
	Home      string
}

type Stats struct {
	Files       int `json:"arquivos"`
	Claims      int `json:"afirmacoes"`
	Findings    int `json:"achados"`
	Inactive    int `json:"nota_inativa"`
	Historical  int `json:"historico"`
	Negated     int `json:"negado"`
	Placeholder int `json:"placeholder"`
	Ignored     int `json:"gerado"`
	Unresolved  int `json:"sem_ancora"`
	OutOfScope  int `json:"fora_de_escopo"`
}

var generated = map[string]bool{
	"node_modules": true, "dist": true, "build": true, "out": true, "target": true,
	"vendor": true, ".venv": true, "venv": true, "__pycache__": true, "_site": true,
	"coverage": true, ".next": true, ".nuxt": true, ".turbo": true,
}

const maxIndexed = 200_000

type root struct {
	dir     string
	ignore  *gitignore.Matcher
	index   map[string][]string
	indexed bool
	subdirs []string
	listed  bool
}

type ghResult struct {
	reason string
}

type Checker struct {
	opt    Options
	roots  map[string]*root
	gh     map[string]ghResult
	ghDown string
	client *http.Client
	Stats  Stats
}

func New(opt Options) *Checker {
	if opt.Home == "" {
		opt.Home, _ = os.UserHomeDir()
	}
	if opt.GitHubAPI == "" {
		opt.GitHubAPI = "https://api.github.com"
	}
	return &Checker{
		opt:    opt,
		roots:  map[string]*root{},
		gh:     map[string]ghResult{},
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Checker) File(file string, content []byte) []Finding {
	c.Stats.Files++
	doc := extract.Parse(file, content)
	fileDir := filepath.Dir(file)
	r := c.rootFor(fileDir, doc.Root)

	var out []Finding
	for _, cl := range doc.Claims {
		c.Stats.Claims++
		switch {
		case doc.Inactive && !c.opt.History:
			c.Stats.Inactive++
			continue
		case cl.Placeholder:
			c.Stats.Placeholder++
			continue
		case cl.Historical && !c.opt.History:
			c.Stats.Historical++
			continue
		case cl.Negated && !c.opt.History:
			c.Stats.Negated++
			continue
		}
		var f *Finding
		if cl.Kind == extract.KindGitHub {
			f = c.github(cl)
		} else {
			f = c.path(cl, fileDir, r)
		}
		if f != nil {
			c.Stats.Findings++
			out = append(out, *f)
		}
	}
	return out
}

func (c *Checker) path(cl extract.Claim, fileDir string, r *root) *Finding {
	v := cl.Value
	isDir := strings.HasSuffix(v, "/")

	if strings.HasPrefix(v, "~/") || (strings.HasPrefix(v, "/") && cl.Kind == extract.KindPath) {
		p := c.expandHome(v)

		if !c.opt.CheckHome || c.opt.Home == "" || !within(p, c.opt.Home) {
			c.Stats.OutOfScope++
			return nil
		}
		if exists(p) {
			return nil
		}
		return &Finding{Claim: cl, Reason: "caminho absoluto não existe"}
	}

	var candidates []string
	if strings.HasPrefix(v, "/") {
		candidates = append(candidates, filepath.Join(r.dir, v))
	} else {
		candidates = append(candidates, filepath.Join(fileDir, v), filepath.Join(r.dir, v))

		for _, sub := range r.children() {
			candidates = append(candidates, filepath.Join(sub, v))
		}
		for _, a := range c.opt.Also {
			candidates = append(candidates, filepath.Join(a, v))
		}
	}
	for _, p := range candidates {
		if exists(p) {
			return nil
		}
	}

	rel := strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(v)), "/")
	if isGenerated(rel) || r.ignore.Match(rel, isDir) {
		c.Stats.Ignored++
		return nil
	}
	if !c.opt.Strict && !c.anchored(rel, fileDir, r) {
		c.Stats.Unresolved++
		return nil
	}
	reason := "caminho não existe"
	if cl.Kind == extract.KindLink {
		reason = "destino do link não existe"
	}
	return &Finding{Claim: cl, Reason: reason, Root: r.dir, Suggestions: r.suggest(rel)}
}

func (c *Checker) anchored(rel, fileDir string, r *root) bool {
	first := strings.SplitN(rel, "/", 2)[0]
	if first == ".." || exists(filepath.Join(fileDir, first)) || exists(filepath.Join(r.dir, first)) {
		return true
	}
	for _, d := range append(r.children(), c.opt.Also...) {
		if exists(filepath.Join(d, first)) {
			return true
		}
	}
	return false
}

func (c *Checker) github(cl extract.Claim) *Finding {
	if !c.opt.GitHub {
		c.Stats.OutOfScope++
		return nil
	}
	key := strings.ToLower(cl.Value)
	res, ok := c.gh[key]
	if !ok {
		if c.ghDown != "" {
			c.Stats.OutOfScope++
			return nil
		}
		res = c.lookupGitHub(cl.Value)
		if c.ghDown != "" {
			c.Stats.OutOfScope++
			return nil
		}
		c.gh[key] = res
	}
	if res.reason == "" {
		return nil
	}
	return &Finding{Claim: cl, Reason: res.reason}
}

func (c *Checker) lookupGitHub(repo string) ghResult {
	req, err := http.NewRequest(http.MethodGet, c.opt.GitHubAPI+"/repos/"+repo, nil)
	if err != nil {
		return ghResult{}
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.opt.GitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.opt.GitHubToken)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		c.ghDown = err.Error()
		return ghResult{}
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		var body struct {
			FullName string `json:"full_name"`
		}
		if json.NewDecoder(resp.Body).Decode(&body) == nil && !strings.EqualFold(body.FullName, repo) {

			return ghResult{reason: "repositório no GitHub foi renomeado para " + body.FullName}
		}
		return ghResult{}
	case http.StatusNotFound:
		return ghResult{reason: "repositório no GitHub não encontrado (ou privado sem acesso)"}
	default:
		c.ghDown = fmt.Sprintf("a API do GitHub respondeu %s", resp.Status)
		return ghResult{}
	}
}

func (c *Checker) GitHubUnavailable() string { return c.ghDown }

func (c *Checker) rootFor(fileDir, declared string) *root {
	dir := ""
	if declared != "" {
		if d := c.expandHome(declared); isDirPath(d) {
			dir = d
		}
	}
	if dir == "" {
		dir = gitRoot(fileDir)
	}
	if r, ok := c.roots[dir]; ok {
		return r
	}
	r := &root{dir: dir, ignore: gitignore.Load(filepath.Join(dir, ".gitignore"))}
	c.roots[dir] = r
	return r
}

func (c *Checker) expandHome(p string) string {
	if strings.HasPrefix(p, "~/") && c.opt.Home != "" {
		return filepath.Join(c.opt.Home, p[2:])
	}
	return p
}

func (r *root) children() []string {
	if r.listed {
		return r.subdirs
	}
	r.listed = true
	entries, _ := os.ReadDir(r.dir)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() && !strings.HasPrefix(name, ".") && !generated[name] && !r.ignore.Match(name, true) {
			r.subdirs = append(r.subdirs, filepath.Join(r.dir, name))
		}
	}
	return r.subdirs
}

func (r *root) suggest(rel string) []string {
	base := path.Base(strings.TrimSuffix(rel, "/"))
	if base == "." || base == "/" {
		return nil
	}
	r.buildIndex()
	cands := r.index[base]
	if len(cands) == 0 {
		return nil
	}
	type scored struct {
		p     string
		score int
	}
	want := strings.Split(strings.TrimSuffix(rel, "/"), "/")
	var list []scored
	for _, p := range cands {
		list = append(list, scored{p, commonSuffix(want, strings.Split(p, "/"))})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		return len(list[i].p) < len(list[j].p)
	})

	if list[0].score <= 1 && len(list) > 3 {
		return nil
	}
	var out []string
	for i := 0; i < len(list) && i < 3; i++ {
		out = append(out, list[i].p)
	}
	return out
}

func (r *root) buildIndex() {
	if r.indexed {
		return
	}
	r.indexed = true
	r.index = map[string][]string{}
	n := 0
	_ = filepath.WalkDir(r.dir, func(p string, d fs.DirEntry, err error) error {
		if n >= maxIndexed {
			return fs.SkipAll
		}
		if err != nil {
			return nil
		}
		if p == r.dir {
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(p, r.dir+string(filepath.Separator)))
		name := d.Name()
		if d.IsDir() && (strings.HasPrefix(name, ".") || generated[name] || r.ignore.Match(rel, true)) {
			return fs.SkipDir
		}
		if !d.IsDir() && r.ignore.Match(rel, false) {
			return nil
		}
		r.index[name] = append(r.index[name], rel)
		n++
		return nil
	})
}

func commonSuffix(a, b []string) int {
	n := 0
	for i, j := len(a)-1, len(b)-1; i >= 0 && j >= 0 && a[i] == b[j]; i, j = i-1, j-1 {
		n++
	}
	return n
}

func isGenerated(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if generated[seg] {
			return true
		}
	}
	return false
}

func gitRoot(dir string) string {
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return dir
		}
		d = parent
	}
}

func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isDirPath(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
