# robotstxt

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

Depends on `cl0q/botid` (bot names). For local work, clone it as a sibling directory
(the go.mod `replace` points at `../botid`).

Tests: `go test -race ./...`
