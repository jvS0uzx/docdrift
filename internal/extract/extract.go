package extract

import (
	"bufio"
	"bytes"
	"net/url"
	"path"
	"regexp"
	"strings"
)

type Kind string

const (
	KindPath Kind = "path"

	KindLink   Kind = "link"
	KindGitHub Kind = "github"
)

type Claim struct {
	File        string `json:"file"`
	Line        int    `json:"line"`
	Kind        Kind   `json:"kind"`
	Value       string `json:"value"`
	Historical  bool   `json:"historical,omitempty"`
	Negated     bool   `json:"negated,omitempty"`
	Placeholder bool   `json:"placeholder,omitempty"`
}

type Doc struct {
	Claims []Claim

	Root     string
	Inactive bool
}

const (
	ignoreLine = "<!-- docdrift:ignore -->"
	ignoreFile = "<!-- docdrift:ignore-file -->"
)

var (
	inlineCode = regexp.MustCompile("`([^`\n]+)`")
	mdLink     = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	githubRef  = regexp.MustCompile(`github\.com/([A-Za-z0-9-]+)/([A-Za-z0-9._-]+)`)
	pathLike   = regexp.MustCompile(`\S*/\S*`)
	lineRef    = regexp.MustCompile(`:\d+(?::\d+)?$`)
	rootInFM   = regexp.MustCompile(`(?:^|\s)((?:/|~/)\S+)`)

	historical    = regexp.MustCompile(`(?i)\b(antig[oa]s?|renomead[oa]s?|era|eram|removid[oa]s?|apagad[oa]s?|movid[oa]s?|hist[óo]ric[oa]s?|deprecad[oa]s?|substitu[íi]d[oa]s?|migrad[oa]s?|antes|formerly|renamed|removed|deleted|deprecated|legacy|previously|used to|no longer)\b`)
	negated       = regexp.MustCompile(`(?i)n[ãa]o\s+existe|n[ãa]o\s+h[áa](\s|$)|inexistente|ausente|\bmort[oa]s?\b|\b404\b|❌|does\s+not\s+exist|doesn't\s+exist|\bmissing\b|\bgone\b`)
	inactive      = regexp.MustCompile(`(?i)desativad|arquivad|hist[óo]ric|obsolet|descontinuad|abandonad|deprecated|archived|inactive`)
	placeholderCI = regexp.MustCompile(`(?i)seu[-_]?usuario|seu[-_]?repo|your[-_]?(user(name)?|org|repo|name)|\b(owner|user|org|dono|usuario)/(repo|name|nome|repositorio)\b|<[^>]+>|\{[^}]+\}|\[[^\]]+\]|(^|/)\.\.\.(/|$)`)

	placeholderUpper = regexp.MustCompile(`NNN|XXX|AAAA|YYYY|HHMMSS`)

	githubReserved = map[string]bool{
		"orgs": true, "settings": true, "features": true, "sponsors": true, "marketplace": true,
		"topics": true, "apps": true, "login": true, "about": true, "pricing": true,
		"enterprise": true, "site": true, "notifications": true, "new": true,
	}

	knownExt = map[string]bool{}
)

func init() {
	for _, e := range strings.Fields(`go mod sum ts tsx js jsx mjs cjs py rb rs java kt kts swift c h cc cpp hpp cs php
		sql sh bash zsh fish ps1 yml yaml json jsonc toml ini cfg conf env example md mdx txt rst html css scss sass
		vue svelte astro lock service timer socket canvas xml gradle tf hcl proto graphql gql csv tsv ipynb dart lua
		ex exs erl zig nix dockerfile svg png jpg jpeg gif webp pdf`) {
		knownExt[e] = true
	}
}

func Parse(file string, content []byte) Doc {
	var doc Doc
	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	n, inFence, inFrontmatter, fmKey := 0, false, false, ""
	for sc.Scan() {
		n++
		line := sc.Text()
		trimmed := strings.TrimSpace(line)

		if n == 1 && trimmed == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter {
			switch {
			case trimmed == "---":
				inFrontmatter = false
			case strings.HasPrefix(trimmed, "- "):
				if (fmKey == "status" || fmKey == "tags") && inactive.MatchString(trimmed) {
					doc.Inactive = true
				}
			default:
				key, value, _ := strings.Cut(trimmed, ":")
				fmKey = strings.ToLower(strings.TrimSpace(key))
				switch fmKey {
				case "repo":
					if m := rootInFM.FindStringSubmatch(value); m != nil {
						doc.Root = m[1]
					}
				case "status", "tags":
					if inactive.MatchString(value) {
						doc.Inactive = true
					}
				}
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence {
			prose := inlineCode.ReplaceAllString(line, "")
			if strings.Contains(prose, ignoreFile) {
				return Doc{}
			}
			if strings.Contains(prose, ignoreLine) {
				continue
			}
		}

		words := pathLike.ReplaceAllString(inlineCode.ReplaceAllString(line, " "), " ")
		hist := historical.MatchString(words)
		neg := negated.MatchString(words)
		add := func(k Kind, v string) {
			doc.Claims = append(doc.Claims, Claim{
				File: file, Line: n, Kind: k, Value: v, Historical: hist,
				Negated: neg, Placeholder: isPlaceholder(v),
			})
		}

		if !inFence {
			for _, m := range inlineCode.FindAllStringSubmatch(line, -1) {
				if p, ok := PathToken(m[1]); ok {
					add(KindPath, p)
				}
			}
			for _, m := range mdLink.FindAllStringSubmatch(line, -1) {
				if p, ok := linkTarget(m[1]); ok {
					add(KindLink, p)
				}
			}
		}
		for _, m := range githubRef.FindAllStringSubmatch(line, -1) {
			owner := m[1]
			repo := strings.TrimSuffix(strings.TrimRight(m[2], "."), ".git")
			if githubReserved[strings.ToLower(owner)] || repo == "" {
				continue
			}
			add(KindGitHub, owner+"/"+repo)
		}
	}
	return doc
}

func PathToken(tok string) (string, bool) {
	tok = strings.TrimSpace(tok)
	tok = lineRef.ReplaceAllString(tok, "")
	if tok == "" || strings.Contains(tok, "://") || strings.ContainsAny(tok, "\\@:*?$|=;,\"'`<>") {
		return "", false
	}
	abs := strings.HasPrefix(tok, "/") || strings.HasPrefix(tok, "~/")

	if !abs && strings.ContainsAny(tok, " \t") {
		return "", false
	}
	if strings.HasPrefix(tok, "-") {
		return "", false
	}
	tok = strings.TrimPrefix(tok, "./")
	if !strings.Contains(strings.TrimSuffix(tok, "/"), "/") && !abs {
		return "", false
	}
	if abs {
		return tok, len(tok) > 2
	}
	if strings.HasSuffix(tok, "/") || hasKnownExt(tok) {
		return tok, true
	}
	return "", false
}

func hasKnownExt(p string) bool {
	ext := strings.TrimPrefix(path.Ext(p), ".")
	return ext != "" && knownExt[strings.ToLower(ext)]
}

func linkTarget(raw string) (string, bool) {
	if raw == "" || strings.HasPrefix(raw, "#") || strings.Contains(raw, "://") ||
		strings.HasPrefix(raw, "mailto:") || strings.HasPrefix(raw, "obsidian:") || strings.HasPrefix(raw, "tel:") {
		return "", false
	}
	if i := strings.IndexAny(raw, "#?"); i >= 0 {
		raw = raw[:i]
	}
	if dec, err := url.PathUnescape(raw); err == nil {
		raw = dec
	}
	raw = strings.TrimPrefix(raw, "<")
	raw = strings.TrimSuffix(raw, ">")
	return raw, raw != ""
}

func isPlaceholder(v string) bool {
	return placeholderCI.MatchString(v) || placeholderUpper.MatchString(v)
}
