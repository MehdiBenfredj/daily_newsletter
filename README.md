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

Rating weights are configured with the `*_COEF` values in `.env`; they must sum
to `1.0`. `RANDOMNESS_FACTOR` adds a small random multiplier after scoring. For
example, `RANDOMNESS_FACTOR=15%` changes each score by a random factor from
`0.85` to `1.15`. Use `0%` for deterministic scores.

```sh
go run ./cmd/publish-newsletter
```

The command reads `site/sources.json`, fetches configured RSS/website/API sources,
parses each source into information items, enriches them with source metadata,
rates them with OpenRouter, sorts them by rating, and writes
`processed_informations.json`.

Each output information item keeps the parsed article fields (`url`, `title`,
`date_published`, `description`) and adds publishing metadata such as `index`,
`source`, `theme`, `personal_preference`, and `rating`.

To use a custom source file:

```sh
go run ./cmd/publish-newsletter --sources path/to/sources.json
```

## Tests

```sh
go test ./...
```
