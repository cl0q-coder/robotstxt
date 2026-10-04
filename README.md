# robotstxt

![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)
![Go 1.25](https://img.shields.io/badge/Go-1.25-00ADD8.svg)

An RFC 9309 robots.txt parser used by the cl0q crawlers. Honours the group that names
our bot (via `cl0q/botid`) ahead of `*`, longest-match Allow/Disallow with `*` and `$`
wildcards, and a Crawl-delay capped to a sane maximum. Built to fail safe: a missing or
unreachable robots.txt allows crawling; a parse it can't understand never widens access.

```go
r := robotstxt.Parse(body)                    // *Rules
r.Allows(u)         // bool, u is a *url.URL
r.AllowsPath("/private/page")                 // bool, path form
r.Delay()           // time.Duration, Crawl-delay capped to a sane max
```

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

## Configuration

None — this is a library. Behavior is set by the robots.txt body it parses.

## Related repositories

- [botid](../botid) — the bot names this parser matches against.
- [crawler-node](../crawler-node) — the crawler that obeys these rules (vendored as `robotstxt/`).
- [netguard](../netguard) — the SSRF guard on the crawler's dialer.

Tests: `go test -race ./...`. MIT licensed.
