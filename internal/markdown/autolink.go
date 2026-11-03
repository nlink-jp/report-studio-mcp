package markdown

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// GFM 0.29-gfm §6.9 extended autolinks. www. and http(s):// links are found
// while inlines are parsed, as cmark-gfm does, so that a path such as
// "/a_b_c" is not first taken apart as emphasis; email and mailto:/xmpp:
// links are found afterwards in the merged text, outside links.

// parseExtAutolink tries a www. or http(s):// autolink at ip.pos.
func (ip *inlineParser) parseExtAutolink() bool {
	s := ip.subj[ip.pos:]
	var prefixLen int
	var scheme string
	switch {
	case strings.HasPrefix(s, "www."):
		prefixLen = 4
		scheme = "http://"
	case hasPrefixFold(s, "http://"):
		prefixLen = 7
	case hasPrefixFold(s, "https://"):
		prefixLen = 8
	default:
		return false
	}
	domainFrom := prefixLen
	if scheme != "" {
		domainFrom = 0 // "www." is the domain's first segment
	}
	d := scanDomain(s, domainFrom, prefixLen)
	if d < 0 || defangedAfter(s, d) {
		return false
	}
	e := d
	for e < len(s) && !isWhitespaceByte(s[e]) && s[e] != '<' {
		e++
	}
	e = autolinkTrim(s[:e])
	if e <= prefixLen {
		return false
	}
	text := s[:e]
	ip.addAutolink(normalizeURI(scheme+text), text)
	ip.pos += e
	return true
}

// defangedAfter reports whether the domain that ends at s[end] runs on into
// a bracket or parenthesis, as defanged indicators do in security reports
// ("evil[.]com", "evil(.)com", "user[@]host"). Such a domain is not made a
// link: the link would point at a truncated or different host
// ("user@mail.evil[.]com" would become mailto:user@mail.evil). Explicit links
// and <...> autolinks are not affected.
func defangedAfter(s string, end int) bool {
	return end < len(s) && (s[end] == '[' || s[end] == '(')
}

// scanDomain reads a valid domain starting at s[from]; at least one domain
// character must follow s[need]. Segments of alphanumerics, '-' and '_' are
// separated by '.', and the last two segments hold no '_'. It returns the
// end of the domain, or -1.
func scanDomain(s string, from, need int) int {
	j := from
	curUnderscore, prevUnderscore := false, false
	for j < len(s) {
		c := s[j]
		switch {
		case c == '.':
			prevUnderscore, curUnderscore = curUnderscore, false
			j++
			continue
		case c == '_':
			curUnderscore = true
		case isAlnum(c) || c == '-':
		case c >= utf8.RuneSelf:
			r, w := utf8.DecodeRuneInString(s[j:])
			if !(unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)) {
				return domainEnd(j, need, s, curUnderscore, prevUnderscore)
			}
			j += w
			continue
		default:
			return domainEnd(j, need, s, curUnderscore, prevUnderscore)
		}
		j++
	}
	return domainEnd(j, need, s, curUnderscore, prevUnderscore)
}

func domainEnd(j, need int, s string, cur, prev bool) int {
	if j <= need || s[need] == '.' || cur || prev {
		return -1
	}
	return j
}

// autolinkTrim drops what GFM excludes from the end of an extended
// autolink: trailing punctuation, an entity-like "&name;", and closing
// parentheses beyond the opening ones. Parentheses are counted once, not per
// character removed (cmark-gfm's per-character recount was quadratic).
func autolinkTrim(s string) int {
	end := len(s)
	opens, closes := strings.Count(s, "("), strings.Count(s, ")")
	for end > 0 {
		c := s[end-1]
		switch {
		case strings.IndexByte("?!.,:*_~'\"", c) >= 0:
			end--
		case c == ';':
			k := end - 2
			for k >= 0 && isAlnum(s[k]) {
				k--
			}
			if k >= 0 && k < end-2 && s[k] == '&' {
				end = k
			} else {
				end--
			}
		case c == ')' && closes > opens:
			closes--
			end--
		default:
			return end
		}
	}
	return end
}

// postprocessEmails links email addresses in text outside links and images.
func postprocessEmails(root *inode) {
	stack := []*inode{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for c := n.first; c != nil; {
			switch c.kind {
			case iEmph, iStrong, iStrike, iGroup:
				stack = append(stack, c)
				c = c.next
			case iText:
				last, at := c, strings.IndexByte(c.lit, '@') >= 0
				for last.next != nil && last.next.kind == iText {
					last = last.next
					at = at || strings.IndexByte(last.lit, '@') >= 0
				}
				next := last.next
				if at {
					linkEmails(c, last)
				}
				c = next
			default:
				c = c.next
			}
		}
	}
}

func linkEmails(first, last *inode) {
	var b strings.Builder
	for t := first; ; t = t.next {
		b.WriteString(t.lit)
		if t == last {
			break
		}
	}
	text := b.String()
	spans := findEmails(text)
	if len(spans) == 0 {
		return
	}
	prev := 0
	for _, sp := range spans {
		if sp.start > prev {
			first.insertBefore(&inode{kind: iText, lit: text[prev:sp.start]})
		}
		l := &inode{kind: iLink, dest: normalizeURI(sp.dest), autolink: true, depth: 1}
		l.appendChild(&inode{kind: iText, lit: text[sp.start:sp.end]})
		first.insertBefore(l)
		prev = sp.end
	}
	if prev < len(text) {
		first.insertBefore(&inode{kind: iText, lit: text[prev:]})
	}
	for t := first; ; {
		next := t.next
		t.unlink()
		if t == last {
			break
		}
		t = next
	}
}

type emailSpan struct {
	start, end int
	dest       string
}

func isEmailLocal(c byte) bool {
	return isAlnum(c) || c == '.' || c == '+' || c == '-' || c == '_'
}

// findEmails finds GFM extended email and mailto:/xmpp: autolinks. Each
// scan stops at the neighbouring '@', so the whole pass is linear.
func findEmails(t string) []emailSpan {
	var spans []emailSpan
	floor := 0 // end of the previous link
	for from := 0; from < len(t); {
		k := strings.IndexByte(t[from:], '@')
		if k < 0 {
			break
		}
		at := from + k
		from = at + 1
		s := at
		for s > floor && isEmailLocal(t[s-1]) {
			s--
		}
		if s == at {
			continue
		}
		start, proto := s, ""
		if s-1 >= floor && t[s-1] == ':' {
			for _, p := range []string{"mailto", "xmpp"} {
				ps := s - 1 - len(p)
				if ps >= floor && t[ps:s-1] == p && (ps == floor || !isAlnum(t[ps-1])) {
					start, proto = ps, p
					break
				}
			}
		}
		e, np := at+1, 0
		if e >= len(t) || !(isAlnum(t[e]) || t[e] == '-' || t[e] == '_') {
			continue
		}
		for e < len(t) {
			c := t[e]
			if isAlnum(c) || c == '-' || c == '_' {
				e++
			} else if c == '.' && e+1 < len(t) && isAlnum(t[e+1]) {
				np++
				e++
			} else {
				break
			}
		}
		if np == 0 || t[e-1] == '-' || t[e-1] == '_' || e < len(t) && t[e] == '@' {
			continue
		}
		// The domain loop stops before a '.' that no alphanumeric follows,
		// so "a@mail.evil.[.]com" ends at "evil" with ".[" after it.
		if k := e + strings.IndexFunc(t[e:], func(r rune) bool { return r != '.' }); k >= e && defangedAfter(t, k) {
			continue
		}
		if proto == "xmpp" && e+1 < len(t) && t[e] == '/' {
			r := e + 1
			for r < len(t) && (isAlnum(t[r]) || t[r] == '@' || t[r] == '.') {
				r++
			}
			if r > e+1 {
				e = r
			}
		}
		e = start + autolinkTrim(t[start:e])
		if e <= at+1 {
			continue
		}
		dest := t[start:e]
		if proto == "" {
			dest = "mailto:" + dest
		}
		spans = append(spans, emailSpan{start: start, end: e, dest: dest})
		floor = e
		from = e
	}
	return spans
}
