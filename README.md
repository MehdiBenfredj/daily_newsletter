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
`LOG_DIR` controls Go publisher logs, and `LOG_DIR_POPULATE` controls the
TypeScript site-population logs.

## Observability

The Go publisher exports OpenTelemetry traces, metrics, and logs over OTLP when
`OTEL_SDK_DISABLED` is not `true`. Local text logs still go to stdout and
`LOG_DIR`; OpenTelemetry logs are an additional exported copy that can be
correlated with traces by trace/span IDs.

publish:

```sh
scripts/publish_newsletter.sh
```

By default `.env.template` points the publisher at `http://localhost:4317`.
To send telemetry to a hosted backend later, keep the application settings the
same and replace or extend the collector exporters in
`otel-collector-config.yaml`.

Rating weights are configured with the `*_COEF` values in `.env`; they must sum
to `1.0`. `RANDOMNESS_FACTOR` adds a small random multiplier after scoring. For
example, `RANDOMNESS_FACTOR=15%` changes each score by a random factor from
`0.85` to `1.15`. Use `0%` for deterministic scores.

```sh
scripts/publish_newsletter.sh
```

Publishing starts with `scripts/publish_newsletter.sh`. That script first calls
`scripts/run_go_publisher.sh "$@"`, then calls `scripts/populate_site.sh`.

`scripts/run_go_publisher.sh` runs the Go publisher, which reads
`site/sources.json`, fetches configured RSS/website/API sources, parses each
source into information items, enriches them with source metadata, rates them
with OpenRouter, sorts them by rating, and writes `processed_informations.json`.
`scripts/populate_site.sh` then populates the static site from the processed
newsletter output. The populate script loads `.env` before running Node so
`LOG_DIR_POPULATE` is available when publishing through the shell scripts.

Each output information item keeps the parsed article fields (`url`, `title`,
`date_published`, `description`) and adds publishing metadata such as `index`,
`source`, `theme`, `personal_preference`, and `rating`.

To use a custom source file:

```sh
scripts/publish_newsletter.sh --sources path/to/sources.json
```

## Tests

```sh
go test ./...
```
