# Daily Newsletter

Static archive for Mehdi's daily curated newsletter.

- Built as plain HTML/CSS.
- Deployed to GitHub Pages with GitHub Actions.
- No Ruby/Jekyll. `.nojekyll` is included.
- Daily source collection is handled by an idiomatic Go CLI.

## Structure

```text
site/
  index.html              # latest newsletter
  archive/YYYY-MM-DD.html # daily snapshots
  archive.json            # machine-readable archive index
.github/workflows/pages.yml
```

## Publishing

```sh
go run ./cmd/publish-newsletter
```

The command reads `site/sources.json`, fetches configured RSS/website/API sources,
prints per-source information counts, and writes `processed_sources.json`.

To use a custom source file:

```sh
go run ./cmd/publish-newsletter --sources path/to/sources.json
```

## Tests

```sh
go test ./...
```
