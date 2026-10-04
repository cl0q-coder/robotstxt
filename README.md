# robotstxt

![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)
![Go 1.25](https://img.shields.io/badge/Go-1.25-00ADD8.svg)

An RFC 9309 robots.txt parser used by the cl0q crawlers. Honours the group that names
our bot (via `cl0q/botid`) ahead of `*`, longest-match Allow/Disallow with `*` and `$`
wildcards, and a Crawl-delay capped to a sane maximum. Built to fail safe: a missing or
unreachable robots.txt allows crawling; a parse it can't understand never widens access.

## Installation

```bash
go get cl0q/robotstxt
```

Requires Go 1.25+. Depends on `cl0q/botid` for bot name matching.

## Quick start

```bash
# clone this repo and botid as sibling directories (go.mod has replace ../botid)
cd robotstxt && go test -race ./...
go build ./...
```

Depends on `cl0q/botid` (bot names). For local work, clone it as a sibling directory
(the go.mod `replace` points at `../botid`).

## How it works

One file (`robotstxt.go`), no dependencies beyond `cl0q/botid`:

- `Parse(body)` groups rules by user-agent, preferring the group that names one
  of our bots over the `*` group.
- `AllowsPath` applies longest-match-wins across Allow/Disallow rules; plain
  (wildcard-free) patterns take a `strings.HasPrefix` fast path instead of the
  compiled regexp.
- `Delay()` returns the group's Crawl-delay, capped to a sane maximum.

## Usage

```go
import "cl0q/robotstxt"

// Parse a robots.txt body
r := robotstxt.Parse(body)  // *Rules

// Check if a URL is allowed
r.Allows(u)                   // bool, u is a *url.URL
r.AllowsPath("/private/page") // bool, path form

// Get the crawl delay (capped to a sane maximum)
r.Delay() // time.Duration
```

### Example

```go
body := `User-agent: *
Disallow: /private/
Allow: /private/public/

User-agent: cl0q-crawler
Disallow: /`

r := robotstxt.Parse(body)
// the cl0q-crawler group names our bot (via botid), so it wins over *:
r.AllowsPath("/private/public") // false (cl0q-crawler group disallows /)
```

## API Reference

### `Parse(body string) *Rules`

Parses a robots.txt body. Never returns nil; on parse error returns rules that
allow everything (fail safe).

### `(*Rules).Allows(u *url.URL) bool`

Reports whether the URL may be crawled. Uses the bot-specific group if one names
us (via `cl0q/botid`), otherwise the `*` group. Longest-match wins for
Allow/Disallow.

### `(*Rules).AllowsPath(path string) bool`

Like `Allows` but takes a path string directly. The path is percent-encoded
before matching (handles non-ASCII correctly).

### `(*Rules).Delay() time.Duration`

Returns the Crawl-delay value, capped to a sane maximum (10 seconds). Returns 0
if no delay is specified.

## RFC 9309 Compliance

- Longest-match Allow/Disallow with `*` and `$` wildcards
- Case-insensitive directives, case-sensitive paths
- Percent-encoding normalized before matching
- UTF-8 BOM stripped
- Crawl-delay respected (capped)

## Configuration

None — this is a library. Behavior is set by the robots.txt body it parses.

## Testing

```bash
go test ./...
go test -race ./...
```

The test suite includes differential tests against Google's reference parser.

## Contributing

Pull requests welcome. Please include test cases for new behavior, especially
edge cases in path matching.

## Related repositories

- [botid](../botid) — the bot names this parser matches against.
- [crawler-node](../crawler-node) — the crawler that obeys these rules (vendored as `robotstxt/`).
- [netguard](../netguard) — the SSRF guard on the crawler's dialer.

## License

MIT. See `LICENSE`.
