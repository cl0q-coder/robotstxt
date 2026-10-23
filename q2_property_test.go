// Q2 (#283): property tests for the robots.txt parser.
//
// Coverage (per the Q2 plan line):
//   - groups:        group selection (an ours-group beats *, consecutive
//     User-agent lines share one group, Sitemap/blank lines do
//     not split a group) vs an independent model of the
//     documented selection rule
//   - longest-match: the longest matching rule wins; Allow wins a tie
//   - wildcards:     wildcard.match agrees with a regex oracle on random
//     pattern/path pairs
//   - crawl-delay:   max delay over applicable groups, capped at
//     MaxCrawlDelay; invalid/negative/zero values ignored
//   - 4xx/5xx:       the library's fail-open contract — what callers feed it
//     on 4xx (empty or group-less body) allows everything, and a
//     nil *Rules allows everything. 429/503 throttling is caller
//     policy in crawler-node, not this library.
//   - differential:  random *-group bodies vs the Google reference parser
//     (github.com/jimsmart/grobotstxt) over random probe paths
//
// All generators use fixed seeds, so the tests are deterministic. If any
// property ever fails, that is a real parser bug: do NOT "fix" it here —
// robotstxt production code is RED-tier (Claude's lane). File it on #283.
package robotstxt

import (
	"math/rand"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"codeberg.org/cl0qsearch/botid"
	gro "github.com/jimsmart/grobotstxt"
)

// ---------------------------------------------------------------------------
// Independent model of the documented Parse/Allows semantics.
// ---------------------------------------------------------------------------

type q2ModelRule struct {
	allow bool
	re    *regexp.Regexp
	plen  int // raw pattern length, the same quantity AllowsPath compares
}

type q2ModelGroup struct {
	agents []string
	rules  []q2ModelRule
	delay  time.Duration
}

// q2CompilePattern builds the regex the package comment says wildcard.match
// is byte-for-byte identical to: start-anchored, '*' -> .*, trailing '$'
// anchors the end. It is the oracle, not a copy of the implementation.
func q2CompilePattern(pat string) *regexp.Regexp {
	anchor := strings.HasSuffix(pat, "$")
	p := strings.TrimSuffix(pat, "$")
	re := "^" + strings.ReplaceAll(regexp.QuoteMeta(p), `\*`, ".*")
	if anchor {
		re += "$"
	}
	return regexp.MustCompile(re)
}

// q2ModelParse mirrors the documented group-splitting rule: consecutive
// User-agent lines share one group; anything else starts a new group on the
// next User-agent line; Sitemap:/Host:/blank lines belong to no group and do
// not split one.
func q2ModelParse(body string) []q2ModelGroup {
	var groups []q2ModelGroup
	var cur *q2ModelGroup
	prevField := ""
	for _, line := range strings.Split(body, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		idx := strings.IndexByte(line, ':')
		if line == "" || idx < 0 {
			continue
		}
		field := strings.ToLower(strings.TrimSpace(line[:idx]))
		val := strings.TrimSpace(line[idx+1:])
		switch field {
		case "user-agent":
			if cur == nil || prevField != "user-agent" {
				groups = append(groups, q2ModelGroup{})
				cur = &groups[len(groups)-1]
			}
			cur.agents = append(cur.agents, val)
		case "allow", "disallow":
			if cur != nil && val != "" {
				cur.rules = append(cur.rules, q2ModelRule{
					allow: field == "allow",
					re:    q2CompilePattern(val),
					plen:  len(val),
				})
			}
		case "crawl-delay":
			if cur != nil {
				if f, err := strconv.ParseFloat(val, 64); err == nil && f > 0 {
					cur.delay = time.Duration(min(f, float64(MaxCrawlDelay)/float64(time.Second)) * float64(time.Second))
				}
			}
		}
		switch field {
		case "user-agent", "allow", "disallow", "crawl-delay":
			prevField = field
		}
	}
	return groups
}

// q2ModelVerdict implements the documented selection + matching rule: if any
// group is ours, only ours-groups apply, otherwise only *-groups; among the
// applicable rules the longest match wins and Allow wins a tie; the reported
// delay is the max over applicable groups.
func q2ModelVerdict(groups []q2ModelGroup, path string) (bool, time.Duration) {
	ours := false
	for _, g := range groups {
		for _, a := range g.agents {
			if botid.IsOurs(a) {
				ours = true
			}
		}
	}
	best, allow, delay := -1, true, time.Duration(0)
	for _, g := range groups {
		applies := false
		for _, a := range g.agents {
			if (ours && botid.IsOurs(a)) || (!ours && a == "*") {
				applies = true
			}
		}
		if !applies {
			continue
		}
		if g.delay > delay {
			delay = g.delay
		}
		for _, r := range g.rules {
			if r.plen >= best && r.re.MatchString(path) && (r.plen > best || r.allow) {
				best, allow = r.plen, r.allow
			}
		}
	}
	return allow, delay
}

// q2ProbePath converts a raw probe path the same way Allows does:
// EscapedPath plus the raw query when present.
func q2ProbePath(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse("https://example.com" + raw)
	if err != nil {
		t.Fatalf("probe path %q does not parse: %v", raw, err)
	}
	p := u.EscapedPath()
	if u.RawQuery != "" {
		p += "?" + u.RawQuery
	}
	return p
}

// ---------------------------------------------------------------------------
// Generators (fixed seeds; ASCII-only so escapePattern is the identity and the
// model stays independent of the implementation's percent-encoding).
// ---------------------------------------------------------------------------

var (
	q2PathChars = []rune("abcdefghijklmnopqrstuvwxyz0123456789/.-_~")
	q2PatChars  = []rune("abcdefghijklmnopqrstuvwxyz0123456789/.-_~?=&+()[]{}|^")
	q2Agents    = []string{"*", "cl0q-node", "cl0q-crawler", "cl0q", "googlebot", "otherbot", "CL0Q-NODE", "cl0q-n", "bingbot"}
)

func q2RandPath(rng *rand.Rand) string {
	n := 1 + rng.Intn(12)
	var b strings.Builder
	b.WriteByte('/')
	for i := 0; i < n; i++ {
		b.WriteRune(q2PathChars[rng.Intn(len(q2PathChars))])
	}
	if rng.Intn(4) == 0 {
		b.WriteByte('?')
		for i := 0; i < rng.Intn(6); i++ {
			b.WriteRune(q2PathChars[rng.Intn(len(q2PathChars))])
		}
	}
	return b.String()
}

func q2RandPattern(rng *rand.Rand) string {
	segs := 1 + rng.Intn(4)
	parts := make([]string, segs)
	for i := range parts {
		n := rng.Intn(6)
		var b strings.Builder
		for j := 0; j < n; j++ {
			b.WriteRune(q2PatChars[rng.Intn(len(q2PatChars))])
		}
		parts[i] = b.String()
	}
	p := "/" + strings.Join(parts, "*")
	if rng.Intn(4) == 0 {
		p += "$"
	}
	return p
}

func q2RandBody(rng *rand.Rand) string {
	var b strings.Builder
	for gi, ng := 0, 1+rng.Intn(4); gi < ng; gi++ {
		for ai, na := 0, 1+rng.Intn(3); ai < na; ai++ {
			b.WriteString("User-agent: " + q2Agents[rng.Intn(len(q2Agents))] + "\n")
		}
		for ri, nr := 0, rng.Intn(6); ri < nr; ri++ {
			if rng.Intn(2) == 0 {
				b.WriteString("Allow: ")
			} else {
				b.WriteString("Disallow: ")
			}
			b.WriteString(q2RandPattern(rng) + "\n")
		}
		switch rng.Intn(4) {
		case 0:
			b.WriteString("Crawl-delay: " + strconv.Itoa(rng.Intn(120)) + "\n")
		case 1:
			b.WriteString("Crawl-delay: 1.5\n")
		case 2:
			b.WriteString("Crawl-delay: soon\n")
		default:
			b.WriteString("Crawl-delay: -5\n")
		}
		if rng.Intn(4) == 0 {
			b.WriteString("Sitemap: https://example.com/sitemap.xml\n")
		}
		if rng.Intn(4) == 0 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// q2RandStarBody generates *-group-only bodies so the Google reference parser
// (which knows nothing about cl0q group precedence) selects the same group.
func q2RandStarBody(rng *rand.Rand) string {
	var b strings.Builder
	b.WriteString("User-agent: *\n")
	for ri, nr := 0, 1+rng.Intn(6); ri < nr; ri++ {
		if rng.Intn(2) == 0 {
			b.WriteString("Allow: ")
		} else {
			b.WriteString("Disallow: ")
		}
		b.WriteString(q2RandPattern(rng) + "\n")
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Properties.
// ---------------------------------------------------------------------------

// TestPropertyGroupSelection: over random multi-group bodies, every verdict
// and the reported delay must match the independent model. This exercises
// group precedence (ours beats *), group sharing/splitting, longest-match,
// allow-wins-tie, and delay merging in combination.
func TestPropertyGroupSelection(t *testing.T) {
	rng := rand.New(rand.NewSource(0x5132))
	for i := 0; i < 300; i++ {
		body := q2RandBody(rng)
		r := Parse(body)
		model := q2ModelParse(body)
		var wantDelay time.Duration
		for j := 0; j < 15; j++ {
			p := q2ProbePath(t, q2RandPath(rng))
			wantAllow, d := q2ModelVerdict(model, p)
			wantDelay = d // same for every probe of this body
			if got := r.AllowsPath(p); got != wantAllow {
				t.Errorf("body %d probe %q: got allow=%v, model=%v\nbody:\n%s", i, p, got, wantAllow, body)
			}
		}
		if got := r.Delay(); got != wantDelay {
			t.Errorf("body %d: got delay=%v, model=%v\nbody:\n%s", i, got, wantDelay, body)
		}
	}
}

// TestPropertyLongestMatchTieBreak: single *-group, random rule sets — the
// model isolates longest-match + allow-wins-tie from group selection.
func TestPropertyLongestMatchTieBreak(t *testing.T) {
	rng := rand.New(rand.NewSource(0x7106))
	for i := 0; i < 300; i++ {
		body := q2RandStarBody(rng)
		r := Parse(body)
		model := q2ModelParse(body)
		for j := 0; j < 15; j++ {
			p := q2ProbePath(t, q2RandPath(rng))
			want, _ := q2ModelVerdict(model, p)
			if got := r.AllowsPath(p); got != want {
				t.Errorf("body %d probe %q: got allow=%v, model=%v\nbody:\n%s", i, p, got, want, body)
			}
		}
	}
}

// TestPropertyWildcardMatchesRegexp: the linear wildcard matcher must agree
// with the regex oracle on random pattern/path pairs (deterministic seeded
// sweep complementing FuzzMatcherEquivalence).
func TestPropertyWildcardMatchesRegexp(t *testing.T) {
	rng := rand.New(rand.NewSource(0x9c1d))
	for i := 0; i < 8000; i++ {
		pat := q2RandPattern(rng)
		path := q2RandPath(rng)
		w, ok := compileWildcard(escapePattern(pat))
		if !ok {
			t.Fatalf("pattern %q exceeded wildcard cap (generator bug)", pat)
		}
		if got, want := w.match(path), q2CompilePattern(pat).MatchString(path); got != want {
			t.Errorf("pattern %q path %q: matcher=%v, regexp=%v", pat, path, got, want)
		}
	}
}

// TestPropertyCrawlDelayCap: targeted sweep of the delay contract — values
// pass through verbatim below the cap, the cap binds above it, and
// invalid/zero/negative values are ignored. Applicable-group selection is
// covered by TestPropertyGroupSelection.
func TestPropertyCrawlDelayCap(t *testing.T) {
	for _, tc := range []struct {
		val  string
		want time.Duration
	}{
		{"0.5", 500 * time.Millisecond},
		{"1", time.Second},
		{"1.5", 1500 * time.Millisecond},
		{"29.9", 29900 * time.Millisecond},
		{"30", MaxCrawlDelay},
		{"31", MaxCrawlDelay},
		{"3600", MaxCrawlDelay},
		{"1e9", MaxCrawlDelay},
		{"0", 0},
		{"-5", 0},
		{"soon", 0},
		{"", 0},
		{"1.5.2", 0},
	} {
		r := Parse("User-agent: *\nCrawl-delay: " + tc.val + "\n")
		if got := r.Delay(); got != tc.want {
			t.Errorf("Crawl-delay %q: got %v, want %v", tc.val, got, tc.want)
		}
	}
	// A hostile file cannot stall a worker: delay never exceeds the cap.
	rng := rand.New(rand.NewSource(0xd3cc))
	for i := 0; i < 200; i++ {
		r := Parse(q2RandBody(rng))
		if d := r.Delay(); d < 0 || d > MaxCrawlDelay {
			t.Errorf("delay %v out of [0, MaxCrawlDelay] bounds", d)
		}
	}
}

// TestPropertyFailOpen: the library's side of the 4xx/5xx contract. Callers
// (crawler-node) treat a 4xx robots fetch as "no rules" and 429/503 as
// throttle — both are caller policy. This library must fail open on anything
// that parses to no applicable rules, and a nil *Rules allows everything.
func TestPropertyFailOpen(t *testing.T) {
	probes := []string{"/", "/private/secret", "/a?b=c", "/x.php"}
	var nilRules *Rules
	for _, p := range probes {
		if !nilRules.AllowsPath(p) {
			t.Errorf("nil *Rules must allow %q", p)
		}
		if !Parse("").AllowsPath(p) {
			t.Errorf("empty body (4xx stand-in) must allow %q", p)
		}
		if d := nilRules.Delay(); d != 0 {
			t.Errorf("nil *Rules delay must be 0, got %v", d)
		}
	}
	// An empty Disallow value is skipped (RFC 9309: allows everything).
	if !Parse("User-agent: *\nDisallow:\n").AllowsPath("/private") {
		t.Error("empty Disallow must allow everything")
	}
	// Bodies with no applicable group (neither ours nor *) allow everything.
	rng := rand.New(rand.NewSource(0xfa11))
	others := []string{"googlebot", "bingbot", "otherbot"}
	for i := 0; i < 100; i++ {
		var b strings.Builder
		b.WriteString("User-agent: " + others[rng.Intn(len(others))] + "\n")
		for ri, nr := 0, rng.Intn(5); ri < nr; ri++ {
			b.WriteString("Disallow: " + q2RandPattern(rng) + "\n")
		}
		r := Parse(b.String())
		for j := 0; j < 10; j++ {
			p := q2RandPath(rng)
			if !r.AllowsPath(p) {
				t.Errorf("no applicable group must allow %q\nbody:\n%s", p, b.String())
			}
		}
	}
}

// TestPropertyDifferentialVsGoogle: random *-group bodies vs the Google
// reference parser over random probe paths. Extends the fixed
// testdata/ corpus differential (TestDifferentialVsGoogle) with generated
// cases. Any mismatch is either a real parser bug (file on #283, do not fix
// here — RED-tier) or a documented intentional divergence.
func TestPropertyDifferentialVsGoogle(t *testing.T) {
	rng := rand.New(rand.NewSource(0x60091e))
	for i := 0; i < 200; i++ {
		body := q2RandStarBody(rng)
		r := Parse(body)
		for j := 0; j < 20; j++ {
			raw := q2RandPath(rng)
			u, err := url.Parse("https://example.com" + raw)
			if err != nil {
				t.Fatalf("probe %q does not parse: %v", raw, err)
			}
			uri := u.String()
			if ours, theirs := r.Allows(u), gro.AgentAllowed(body, "*", uri); ours != theirs {
				t.Errorf("body %d probe %q: ours=%v, google=%v\nbody:\n%s", i, raw, ours, theirs, body)
			}
		}
	}
}
