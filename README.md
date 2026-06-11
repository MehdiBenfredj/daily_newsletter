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

Create a local `.env` file from the template before publishing:

```sh
cp .env.template .env
```

Set `OPENROUTER_API_KEY` so the publisher can rate each collected item. The
optional `OPENROUTER_MODEL` value defaults to `openrouter/auto`. Set
`PRIM_API_KEY` when using sources that require Prim API authentication.

```sh
go run ./cmd/publish-newsletter
```

The command reads `site/sources.json`, fetches configured RSS/website/API sources,
prints per-source information counts, rates each output item with OpenRouter,
sorts by rating, and writes `processed_sources.json`.

To use a custom source file:

```sh
go run ./cmd/publish-newsletter --sources path/to/sources.json
```

## Tests

```sh
go test ./...
```
