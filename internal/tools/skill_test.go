package tools

// The bundled report-manuscript skill is installed into an agent's skill
// directory and read as instruction, so a name, directive or caption form in
// it that the server does not have is the agent being told to write something
// the typesetter will not act on. These tests hold the skill to the server in
// both directions (organization ADR-027; RFP Discussion Log 22):
//
//   - every tool the server registers and every built-in profile is named by
//     the skill, and every identifier the skill puts in backticks exists in
//     the server — taken from the registered tools and their schemas, the
//     codes the source raises, the configuration's toml and the results' json
//     tags, and the front-matter keys the parser accepts, not from a copied
//     list. A renamed name therefore fails here once the server drops it;
//   - every directive the skill names parses as a directive;
//   - the example manuscripts parse without a warning, and the caption example
//     is paired with its table by the same rule the typesetters use.

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
	"github.com/nlink-jp/report-studio-mcp/internal/mcpserver"
	"github.com/nlink-jp/report-studio-mcp/internal/placement"
	"github.com/nlink-jp/report-studio-mcp/internal/theme"
	"github.com/nlink-jp/report-studio-mcp/internal/workdir"
)

// skillDir is the skill at the repository root, where the organization's
// validator (tests/validate-skill.sh) and `make package-skill` find it.
const skillDir = "../../report-manuscript"

func readSkill(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("the skill has no SKILL.md: %v", err)
	}
	var more []string
	err = filepath.WalkDir(skillDir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".md") && filepath.Base(p) != "SKILL.md" {
			data, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			more = append(more, string(data))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b) + "\n" + strings.Join(more, "\n")
}

func registeredTools(t *testing.T) []mcpserver.Tool {
	t.Helper()
	srv := mcpserver.New("report-studio-mcp", "test", nil, nil)
	RegisterAll(srv, workdir.Resolver{})
	tools := srv.Tools()
	if len(tools) == 0 {
		t.Fatal("no tools are registered")
	}
	return tools
}

func TestSkillFrontmatter(t *testing.T) {
	s := readSkill(t)
	if !strings.HasPrefix(s, "---\n") {
		t.Fatalf("SKILL.md must start with YAML frontmatter")
	}
	end := strings.Index(s[4:], "\n---")
	if end < 0 {
		t.Fatalf("SKILL.md frontmatter is not terminated")
	}
	front := s[:end+5]
	for _, field := range []string{"name: report-manuscript\n", "description: ", "argument-hint: "} {
		if !strings.Contains(front, field) {
			t.Errorf("frontmatter missing %q", field)
		}
	}
}

func TestSkillNamesEveryToolAndProfile(t *testing.T) {
	s := readSkill(t)
	for _, tool := range registeredTools(t) {
		if !strings.Contains(s, "`"+tool.Name+"`") {
			t.Errorf("the skill does not name the tool %q", tool.Name)
		}
	}
	for _, p := range theme.BuiltinNames() {
		if !strings.Contains(s, "`"+p+"`") {
			t.Errorf("the skill does not describe the built-in profile %q", p)
		}
	}
}

// notServerNames are backticked identifiers the skill uses that are not this
// server's, each with what it is.
var notServerNames = map[string]string{}

var snakeIdent = regexp.MustCompile("`([a-z]+(?:_[a-z0-9]+)+)`")

// keyIdent is the key of a `key: value` example, such as `async: true`: a
// single word, which snakeIdent does not see.
var keyIdent = regexp.MustCompile("`([a-z][a-z0-9_]*): [^`]+`")

// skillIdentifiers returns the names the skill teaches that this test can
// tell from prose: snake_case identifiers in backticks, and the keys of
// backticked `key: value` examples. A single plain word in backticks is not
// among them — it cannot be told from an ordinary word — so a misspelled
// one-word argument or key passes unless an example that parses carries it.
func skillIdentifiers(corpus string) []string {
	var out []string
	for _, re := range []*regexp.Regexp{snakeIdent, keyIdent} {
		for _, m := range re.FindAllStringSubmatch(corpus, -1) {
			out = append(out, m[1])
		}
	}
	return out
}

// A misspelled or retired name passes every "does the skill mention X" check;
// only a closed vocabulary catches it.
func TestSkillNamesOnlyWhatTheServerHas(t *testing.T) {
	known := serverVocabulary(t)
	seen := map[string]bool{}
	for _, name := range skillIdentifiers(readSkill(t)) {
		if seen[name] {
			continue
		}
		seen[name] = true
		if known[name] || acceptedFrontMatterKey(name) {
			continue
		}
		if _, ok := notServerNames[name]; ok {
			continue
		}
		t.Errorf("the skill names `%s`, which is not a tool, argument, code, field or front-matter key of this server; "+
			"fix the skill, or list it in notServerNames with what it is", name)
	}
	if len(seen) < 10 {
		t.Fatalf("found only %d identifiers in the skill; has its form changed?", len(seen))
	}
	for name := range notServerNames {
		if !seen[name] {
			t.Errorf("notServerNames lists %q, which the skill no longer uses", name)
		}
	}
}

// The warnings the skill's table tells the writer how to act on. The list is
// written here; it is checked against the codes the server raises, so it
// cannot name one that does not exist, but a new warning is added by hand.
func TestSkillActsOnTheManuscriptWarnings(t *testing.T) {
	codes := serverCodes(t)
	s := readSkill(t)
	for _, code := range []string{
		"caption_separated", "cell_token_split", "table_wrapped", "mermaid_scaled",
		"mermaid_fallback", "chart_scaled", "chart_labels_thinned", "deep_heading",
		"inline_image", "raw_html", "unknown_front_matter_key", "unclosed_front_matter",
		"duplicate_front_matter_key", "invalid_front_matter_value", "invalid_front_matter_line",
		"link_scheme",
	} {
		if !codes[code] {
			t.Errorf("this test expects the warning %q, which the server no longer raises", code)
			continue
		}
		if !strings.Contains(s, "`"+code+"`") {
			t.Errorf("the skill does not say what to change on %q", code)
		}
	}
}

// The directives are the one thing the skill teaches that the parser reads
// out of an HTML comment; a misspelled one is printed as text.
func TestSkillDirectivesParseAsDirectives(t *testing.T) {
	re := regexp.MustCompile(`<!--\s*([a-z-]+)\s*-->`)
	found := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(readSkill(t), -1) {
		found[m[1]] = true
	}
	if len(found) == 0 {
		t.Fatal("the skill names no directive")
	}
	for name := range found {
		doc := markdown.Parse([]byte("text\n\n<!-- " + name + " -->\n\ntext\n"))
		ok := false
		for _, b := range doc.Children {
			if d, isDir := b.(*markdown.Directive); isDir && d.Kind == name {
				ok = true
			}
		}
		if !ok {
			t.Errorf("the skill teaches <!-- %s -->, which the parser does not read as a directive", name)
		}
	}
	for _, d := range []string{markdown.DirectivePageBreak, markdown.DirectiveKeepWithNext} {
		if !found[d] {
			t.Errorf("the skill does not teach the directive %q", d)
		}
	}
}

// The examples are what a writer copies. They must parse without a warning,
// and a table preceded by a caption must be paired with it.
func TestSkillExampleManuscripts(t *testing.T) {
	block := regexp.MustCompile("(?s)~~~markdown\n(.*?)~~~")
	examples := block.FindAllStringSubmatch(readSkill(t), -1)
	if len(examples) == 0 {
		t.Fatal("the skill has no example manuscript (a ~~~markdown block)")
	}
	captions := 0
	for i, m := range examples {
		doc := markdown.Parse([]byte(m[1]))
		for _, w := range doc.Warnings {
			t.Errorf("example %d, line %d: %s (%s)", i+1, w.Line, w.Code, w.Message)
		}
		items := placement.Items(doc.Children)
		for j, it := range items {
			if _, isTable := it.Block.(*markdown.Table); !isTable || j == 0 {
				continue
			}
			if !items[j-1].Caption {
				t.Errorf("example %d: the table is not paired with a caption", i+1)
				continue
			}
			captions++
		}
	}
	if captions == 0 {
		t.Error("no example shows a caption paired with its table")
	}
}

// acceptedFrontMatterKey asks the parser itself whether it knows the key.
func acceptedFrontMatterKey(key string) bool {
	doc := markdown.Parse([]byte("---\n" + key + ": x\n---\n\ntext\n"))
	for _, w := range doc.Warnings {
		if w.Code == "unknown_front_matter_key" || w.Code == "invalid_front_matter_line" {
			return false
		}
	}
	return true
}

// serverVocabulary is every name the server exposes: its tools, the
// properties of their input schemas, the codes its source raises, and the
// json and toml tags of its own types (results and configuration).
func serverVocabulary(t *testing.T) map[string]bool {
	t.Helper()
	known := map[string]bool{}
	for _, tool := range registeredTools(t) {
		known[tool.Name] = true
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("%s: input schema: %v", tool.Name, err)
		}
		for p := range schema.Properties {
			known[p] = true
		}
	}
	for code := range serverCodes(t) {
		known[code] = true
	}
	tag := regexp.MustCompile(`(?:json|toml):"([a-z0-9_]+)`)
	err := filepath.WalkDir("..", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range tag.FindAllSubmatch(src, -1) {
			known[string(m[1])] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return known
}
