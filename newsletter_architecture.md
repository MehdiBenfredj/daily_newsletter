# Daily Newsletter Script Architecture

There are multiple scripts because the job is split into layers:

- newsletter generation
- publishing to the static GitHub Pages site
- reminder text generation
- shell wrappers for schedulers such as Hermes or cron

This keeps the core newsletter logic separate from deployment and notification concerns.

## Script Roles

### `scripts/personal_newsletter.py`

This is the core engine.

It does the actual newsletter work:

- reads source config from `site/sources.json`
- fetches RSS, Atom, HTML pages, and API feeds
- parses source items into a common item format
- scores items by source tier, keywords, and theme
- filters by recency, language, score, and duplicate titles
- stores collected state in `~/.hermes/newsletter/state.json`
- renders the final HTML newsletter into `~/.hermes/newsletter/output/briefing-YYYY-MM-DD.html`

It exposes two CLI commands:

```bash
python3 scripts/personal_newsletter.py collect
python3 scripts/personal_newsletter.py generate --collect-first
```

`collect` fetches and stores items.

`generate --collect-first` fetches fresh items first, then renders the HTML.

### `scripts/publish_newsletter.py`

This is the publishing layer.

It:

- runs `git pull --rebase origin main`
- runs `personal_newsletter.py generate --collect-first`
- finds the generated HTML in `~/.hermes/newsletter/output/`
- copies it to `site/index.html`
- copies it to `site/archive/YYYY-MM-DD.html`
- updates `site/archive.json`
- commits the changed site files
- pushes to GitHub

This is the script that turns the generated newsletter into the public GitHub Pages site.

### `scripts/personal_newsletter_publish_reminder.py`

This is the publish plus reminder layer.

It:

- runs `publish_newsletter.py`
- imports `personal_newsletter.py`
- calls `recent_items()`
- selects critical must-read items
- prints a Telegram-friendly reminder message

It does not send Telegram itself. It only prints the message. Hermes or another automation layer can forward that output.

### `scripts/publish_newsletter.sh`

This is a thin shell wrapper around `publish_newsletter.py`.

It:

- enables strict shell mode with `set -euo pipefail`
- changes directory to `$HOME/projects/daily_newsletter`
- runs `python3 scripts/publish_newsletter.py`

This exists so a scheduler can call one stable shell command.

### `scripts/personal_newsletter_publish_reminder.sh`

This is the production/scheduler wrapper for publishing and reminder output.

It:

- changes directory to `${NEWSLETTER_REPO:-$HOME/projects/daily_newsletter}`
- loads environment variables from `$HOME/.hermes/.env` if present
- runs `python3 scripts/personal_newsletter_publish_reminder.py`

This is likely the best entrypoint for Hermes because it loads secrets/config first.

## High-Level Architecture

```mermaid
flowchart TD
    A["Scheduler / Hermes / Cron"] --> B{"Which entrypoint?"}

    B --> C["publish_newsletter.sh"]
    B --> D["personal_newsletter_publish_reminder.sh"]

    C --> E["publish_newsletter.py"]
    D --> F["Load ~/.hermes/.env"]
    F --> G["personal_newsletter_publish_reminder.py"]

    G --> E

    E --> H["git pull --rebase"]
    H --> I["personal_newsletter.py generate --collect-first"]

    I --> J["Read site/sources.json"]
    J --> K["Fetch RSS / Atom / HTML / API sources"]
    K --> L["Parse + score + dedupe items"]
    L --> M["Write ~/.hermes/newsletter/state.json"]
    M --> N["Render briefing HTML"]
    N --> O["~/.hermes/newsletter/output/briefing-YYYY-MM-DD.html"]

    O --> P["Copy to site/index.html"]
    O --> Q["Copy to site/archive/YYYY-MM-DD.html"]
    P --> R["Update site/archive.json"]
    Q --> R

    R --> S["git add / commit / push"]
    S --> T["GitHub Pages publishes site"]

    G --> U["Import personal_newsletter.py"]
    U --> V["recent_items()"]
    V --> W["Print Telegram-friendly reminder"]
```

## Data Flow

```mermaid
flowchart LR
    A["site/sources.json"] --> B["Collect sources"]
    B --> C["~/.hermes/newsletter/state.json"]
    C --> D["Filter + score + select items"]
    D --> E["Render HTML"]
    E --> F["~/.hermes/newsletter/output/briefing-YYYY-MM-DD.html"]
    F --> G["site/index.html"]
    F --> H["site/archive/YYYY-MM-DD.html"]
    G --> I["Git commit + push"]
    H --> I
    I --> J["GitHub Pages"]
```

## Responsibility Split

```mermaid
flowchart TB
    subgraph Core["Core newsletter engine"]
        A["personal_newsletter.py"]
    end

    subgraph Publish["Publishing"]
        B["publish_newsletter.py"]
    end

    subgraph Notify["Reminder output"]
        C["personal_newsletter_publish_reminder.py"]
    end

    subgraph Wrappers["Scheduler wrappers"]
        D["publish_newsletter.sh"]
        E["personal_newsletter_publish_reminder.sh"]
    end

    D --> B
    E --> C
    C --> B
    B --> A
    C --> A
```

## Short Version

- `personal_newsletter.py` builds the newsletter.
- `publish_newsletter.py` publishes it to the static site and pushes to GitHub.
- `personal_newsletter_publish_reminder.py` publishes, then prints a reminder message.
- `.sh` files are stable scheduler entrypoints.

The architecture is simple but layered: generation is reusable, publishing is separate, and notification is optional on top.
