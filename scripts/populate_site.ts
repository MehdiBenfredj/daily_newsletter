declare const require: any;
declare const process: any;

const fs = require("fs");
const path = require("path");

type LogAttrs = Record<string, string | number | boolean | undefined>;

type SourceTheme = {
  theme: string;
};

type SourcesConfig = {
  themes: SourceTheme[];
};

type ProcessedInformation = {
  url: string;
  source?: string;
  title: string;
  date_published?: string;
  description?: string;
  image_url?: string;
  theme?: string;
  rating?: number;
};

type SiteArticle = {
  title: string;
  summary: string;
  url: string;
  source: string;
  category: string;
  date: string;
  time: string;
  color: string;
  image?: string;
};

type SiteTheme = {
  id: string;
  label: string;
  articles: SiteArticle[];
};

type SiteData = {
  generatedAt: string;
  home: SiteArticle[];
  themes: SiteTheme[];
};

const repoRoot = path.resolve(process.cwd());
const processedPath = path.join(repoRoot, "processed_informations.json");
const sourcesPath = path.join(repoRoot, "site", "sources.json");
const indexPath = path.join(repoRoot, "site", "index.html");

let logFile: any;

function expandHomeDir(value: string): string {
  if (value === "~") return process.env.HOME || value;
  if (value.startsWith("~/")) return path.join(process.env.HOME || "~", value.slice(2));
  return value;
}

function formatAttrs(attrs: LogAttrs = {}): string {
  return Object.entries(attrs)
    .filter(([, value]) => value !== undefined)
    .map(([key, value]) => `${key}=${JSON.stringify(value)}`)
    .join(" ");
}

function log(level: "INFO" | "WARN" | "ERROR", message: string, attrs: LogAttrs = {}): void {
  const line = `time=${new Date().toISOString()} level=${level}  source=populate_site.ts msg=${JSON.stringify(message)} ${formatAttrs(attrs)}\n`;
  process.stdout.write(line);
  if (logFile) logFile.write(line);
}

function info(message: string, attrs: LogAttrs = {}): void {
  log("INFO", message, attrs);
}

function warn(message: string, attrs: LogAttrs = {}): void {
  log("WARN", message, attrs);
}

function errorLog(message: string, attrs: LogAttrs = {}): void {
  log("ERROR", message, attrs);
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function configureLogging(): void {
  const logDirEnv = "LOG_DIR_POPULATE";
  const rawLogDir = process.env[logDirEnv] || "";
  if (!rawLogDir) {
    warn("log directory env var is not set; logging to stdout only", { env_var: logDirEnv });
    return;
  }

  try {
    const logDir = expandHomeDir(rawLogDir);
    fs.mkdirSync(logDir, { recursive: true });
    const stamp = new Date().toISOString().slice(2, 19).replace("T", "_").replace(/-/g, "-");
    const filePath = path.join(logDir, `${stamp}_daily_newsletter_site.log`);
    logFile = fs.createWriteStream(filePath, { flags: "a" });
    info("logging configured", { log_dir: logDir, log_file: filePath });
  } catch (err) {
    warn("log file setup failed; logging to stdout only", { error: errorMessage(err), log_dir: rawLogDir });
  }
}

function closeLogging(): Promise<void> {
  if (!logFile) return Promise.resolve();
  return new Promise((resolve) => {
    logFile.end(resolve);
  });
}

const colors = [
  "bg-[#ffe4d9] text-[#f05f3e]",
  "bg-[#dceeff] text-[#1871d6]",
  "bg-[#e5f2d9] text-[#4d8b45]",
  "bg-[#eadcff] text-[#7d38e8]",
  "bg-[#ffe0df] text-[#e94f4f]",
  "bg-[#fff0c7] text-[#9a6500]",
  "bg-[#dff5ef] text-[#14795f]",
  "bg-[#e6e8ff] text-[#4250c9]",
];

function readJson<T>(filePath: string): T {
  info("reading json file", { path: filePath });
  const raw = fs.readFileSync(filePath, "utf8");
  info("json file read", { path: filePath, bytes: raw.length });
  return JSON.parse(raw) as T;
}

function slugify(value: string): string {
  const slug = value
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");

  return slug || "theme";
}

function uniqueId(label: string, used: Set<string>): string {
  const base = slugify(label);
  let id = base;
  let counter = 2;

  while (used.has(id)) {
    id = `${base}-${counter}`;
    counter += 1;
  }

  used.add(id);
  return id;
}

function stripHtml(value: string): string {
  return value
    .replace(/<[^>]*>/g, " ")
    .replace(/&nbsp;/g, " ")
    .replace(/&amp;/g, "&")
    .replace(/&quot;/g, '"')
    .replace(/&#39;|&apos;/g, "'")
    .replace(/\s+/g, " ")
    .trim();
}

function decodeHtml(value: string): string {
  return value
    .replace(/&amp;/g, "&")
    .replace(/&quot;/g, '"')
    .replace(/&#39;|&apos;/g, "'")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">");
}

function compactSummary(item: ProcessedInformation): string {
  const clean = stripHtml(item.description || "");
  if (clean) {
    return clean.length > 220 ? `${clean.slice(0, 217).trim()}...` : clean;
  }

  return `A rated pick from ${item.source || "this source"}.`;
}

function displayDate(value: string | undefined): string {
  if (!value) return "Today";

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;

  return new Intl.DateTimeFormat("en", {
    month: "short",
    day: "numeric",
    year: "numeric",
  }).format(date);
}

function sourceLabel(item: ProcessedInformation): string {
  if (item.source) return item.source;

  try {
    return new URL(item.url).hostname.replace(/^www\./, "");
  } catch {
    return "Source";
  }
}

function shouldSkipImagesForItem(item: ProcessedInformation): boolean {
  if ((item.source || "").toLowerCase() === "sortir à paris") return true;

  try {
    return new URL(item.url).hostname.replace(/^www\./, "").toLowerCase() === "sortiraparis.com";
  } catch {
    return false;
  }
}

function absoluteUrl(value: string, baseUrl: string): string | undefined {
  try {
    return new URL(decodeHtml(value), baseUrl).toString();
  } catch {
    return undefined;
  }
}

function isWeakImageCandidate(value: string | undefined): boolean {
  if (!value) return true;

  try {
    const url = new URL(value);
    const host = url.hostname.toLowerCase();
    return host === "lh3.googleusercontent.com" && /(?:^|[?&=/])s0-w300(?:$|[?&=-])/.test(url.search + url.pathname);
  } catch {
    return true;
  }
}

function bestSrcsetCandidate(srcset: string, baseUrl: string): string | undefined {
  const candidates = srcset
    .split(",")
    .map((candidate) => {
      const [rawUrl, rawSize] = candidate.trim().split(/\s+/);
      const width = rawSize?.endsWith("w") ? Number.parseInt(rawSize, 10) : 0;
      return {
        url: absoluteUrl(rawUrl, baseUrl),
        width: Number.isFinite(width) ? width : 0,
      };
    })
    .filter((candidate) => candidate.url);

  candidates.sort((a, b) => b.width - a.width);
  return candidates[0]?.url;
}

function strongImageCandidate(value: string | undefined): string | undefined {
  return value && !isWeakImageCandidate(value) ? value : undefined;
}

function imageFromDescription(item: ProcessedInformation): string | undefined {
  const description = item.description || "";
  const imgTag = description.match(/<img\b[^>]*>/i)?.[0] || "";
  const srcset = imgTag.match(/\bsrcset=["']([^"']+)["']/i)?.[1];
  if (srcset) {
    const image = bestSrcsetCandidate(srcset, item.url);
    if (image) return image;
  }

  const src = imgTag.match(/\b(?:src|data-src)=["']([^"']+)["']/i)?.[1];
  if (src) return absoluteUrl(src, item.url);

  const enclosure = description.match(/https?:\/\/[^\s"'<>]+\.(?:jpe?g|png|webp|gif)(?:\?[^\s"'<>]*)?/i)?.[0];
  return enclosure ? decodeHtml(enclosure) : undefined;
}

async function fetchPageImage(pageUrl: string): Promise<string | undefined> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 2500);

  try {
    info("fetching article page image metadata", { url: pageUrl });
    const response = await fetch(pageUrl, {
      signal: controller.signal,
      redirect: "follow",
      headers: {
        "user-agent": "Mozilla/5.0 (compatible; daily-newsletter-image-resolver/1.0; +https://github.com/MehdiBenfredj/daily_newsletter)",
        accept: "text/html,application/xhtml+xml",
      },
    });
    if (!response.ok) {
      warn("article page image metadata fetch returned non-ok status", { url: pageUrl, status: response.status });
      return undefined;
    }

    const html = await response.text();
    const finalUrl = response.url || pageUrl;
    info("article page image metadata fetched", { url: pageUrl, final_url: finalUrl, bytes: html.length });
    const metaPatterns = [
      /<meta\b(?=[^>]*(?:property|name)=["']og:image(?::secure_url)?["'])(?=[^>]*content=["']([^"']+)["'])[^>]*>/i,
      /<meta\b(?=[^>]*(?:property|name)=["']twitter:image(?::src)?["'])(?=[^>]*content=["']([^"']+)["'])[^>]*>/i,
      /<link\b(?=[^>]*rel=["']image_src["'])(?=[^>]*href=["']([^"']+)["'])[^>]*>/i,
    ];

    for (const pattern of metaPatterns) {
      const image = html.match(pattern)?.[1];
      if (image) {
        const resolved = absoluteUrl(image, finalUrl);
        if (resolved) return resolved;
        warn("article page image metadata had invalid image url", { url: pageUrl, image });
      }
    }
  } catch (err) {
    warn("article page image metadata fetch failed", { url: pageUrl, error: errorMessage(err) });
    return undefined;
  } finally {
    clearTimeout(timeout);
  }

  return undefined;
}

async function imageForItem(item: ProcessedInformation): Promise<string | undefined> {
  if (shouldSkipImagesForItem(item)) {
    info("article image skipped by source exception", { url: item.url, source_name: sourceLabel(item) });
    return undefined;
  }

  const pageImage = strongImageCandidate(await fetchPageImage(item.url));
  if (pageImage) {
    info("article image resolved from page metadata", { url: item.url, source_name: sourceLabel(item), image: pageImage });
    return pageImage;
  }

  const feedImage = item.image_url ? strongImageCandidate(absoluteUrl(item.image_url, item.url)) : undefined;
  if (feedImage) {
    info("article image resolved from feed media metadata", { url: item.url, source_name: sourceLabel(item), image: feedImage });
    return feedImage;
  }

  const descriptionImage = strongImageCandidate(imageFromDescription(item));
  if (descriptionImage) {
    info("article image resolved from description", { url: item.url, source_name: sourceLabel(item), image: descriptionImage });
    return descriptionImage;
  }

  info("article image not found; rendering article without image", { url: item.url, source_name: sourceLabel(item), theme: item.theme || "" });
  return undefined;
}

async function toArticle(item: ProcessedInformation, color: string, section: string): Promise<SiteArticle> {
  info("site article rendering started", { section, url: item.url, source_name: sourceLabel(item), title: item.title });
  return {
    title: item.title,
    summary: compactSummary(item),
    url: item.url,
    source: sourceLabel(item),
    category: item.theme || "General",
    date: displayDate(item.date_published),
    time: typeof item.rating === "number" ? `Rating ${item.rating.toFixed(1)}` : "Rated pick",
    color,
    image: await imageForItem(item),
  };
}

function takeUnique(items: ProcessedInformation[], count: number, used: Set<string>, section: string): ProcessedInformation[] {
  const selected: ProcessedInformation[] = [];
  const sourceCounts = new Map<string, number>();
  let skippedMissing = 0;
  let skippedDuplicate = 0;
  let skippedSourceCap = 0;

  info("selecting section articles", { section, candidates: items.length, target: count });

  for (const item of items) {
    if (!item.url || !item.title) {
      skippedMissing += 1;
      warn("skipping item missing required fields", {
        section,
        url: item.url || "",
        title: item.title || "",
        source_name: item.source || "",
      });
      continue;
    }
    if (used.has(item.url)) {
      skippedDuplicate += 1;
      info("skipping item already used in previous section", { section, url: item.url, source_name: sourceLabel(item), title: item.title });
      continue;
    }
    const source = sourceLabel(item);
    const sourceCount = sourceCounts.get(source) || 0;
    if (sourceCount >= 2) {
      skippedSourceCap += 1;
      info("skipping item due to per-section source cap", { section, url: item.url, source_name: source, title: item.title, source_count: sourceCount });
      continue;
    }

    selected.push(item);
    used.add(item.url);
    sourceCounts.set(source, sourceCount + 1);
    info("selected section article", { section, url: item.url, source_name: source, title: item.title, selected: selected.length });
    if (selected.length === count) break;
  }

  info("section article selection completed", {
    section,
    selected: selected.length,
    candidates: items.length,
    skipped_missing: skippedMissing,
    skipped_duplicate: skippedDuplicate,
    skipped_source_cap: skippedSourceCap,
  });
  return selected;
}

async function toArticles(items: ProcessedInformation[], colorForIndex: (index: number) => string, section: string): Promise<SiteArticle[]> {
  info("rendering section articles", { section, articles: items.length });
  const articles = await Promise.all(items.map(async (item, index) => {
    try {
      const article = await toArticle(item, colorForIndex(index), section);
      info("site article rendered", { section, url: item.url, source_name: sourceLabel(item), title: item.title });
      return article;
    } catch (err) {
      warn("site article rendering failed; using resilient fallback article", {
        section,
        url: item.url,
        source_name: sourceLabel(item),
        title: item.title,
        error: errorMessage(err),
      });
      return {
        title: item.title,
        summary: compactSummary(item),
        url: item.url,
        source: sourceLabel(item),
        category: item.theme || "General",
        date: displayDate(item.date_published),
        time: typeof item.rating === "number" ? `Rating ${item.rating.toFixed(1)}` : "Rated pick",
        color: colorForIndex(index),
      };
    }
  }));
  info("section articles rendered", { section, articles: articles.length });
  return articles;
}

async function buildData(sources: SourcesConfig, processed: ProcessedInformation[]): Promise<SiteData> {
  info("site data build started", { processed_items: processed.length, themes: sources.themes.length });
  const ranked = processed.filter((item) => item.url && item.title);
  const invalidItems = processed.length - ranked.length;
  if (invalidItems > 0) warn("processed items missing required fields; excluding from ranking", { invalid_items: invalidItems });
  const usedUrls = new Set<string>();
  const homeSelection = takeUnique(ranked, 5, usedUrls, "Home");
  const home = await toArticles(homeSelection, (index) => colors[index % colors.length], "Home");
  const themeIds = new Set<string>();

  const themes = await Promise.all(sources.themes.map(async (themeConfig, index) => {
    const label = themeConfig.theme;
    info("theme build started", { theme: label });
    const themeItems = ranked.filter((item) => item.theme === label);
    const themeSelection = takeUnique(themeItems, 5, usedUrls, label);
    const articles = await toArticles(themeSelection, () => colors[index % colors.length], label);
    info("theme build completed", { theme: label, candidates: themeItems.length, articles: articles.length });

    return {
      id: uniqueId(label, themeIds),
      label,
      articles,
    };
  }));

  info("site data build completed", { home_articles: home.length, themes: themes.length });
  return {
    generatedAt: new Date().toISOString(),
    home,
    themes,
  };
}

function serializeForScript(data: SiteData): string {
  return JSON.stringify(data, null, 6).replace(/<\/script/gi, "<\\/script").replace(/<!--/g, "<\\!--");
}

function updateIndex(data: SiteData): void {
  info("site index update started", { path: indexPath });
  const html = fs.readFileSync(indexPath, "utf8");
  info("site index read", { path: indexPath, bytes: html.length });
  const nextData = `const newsletterData = ${serializeForScript(data)};`;
  const nextHtml = html.replace(/const newsletterData = \{[\s\S]*?\n\};/, nextData);

  if (nextHtml === html) {
    throw new Error("Could not find newsletterData block in site/index.html");
  }

  const tempPath = `${indexPath}.tmp`;
  fs.writeFileSync(tempPath, nextHtml);
  fs.renameSync(tempPath, indexPath);
  info("site index update completed", { path: indexPath, bytes: nextHtml.length });
}

function normalizeSources(value: SourcesConfig): SourcesConfig {
  if (!value || !Array.isArray(value.themes)) {
    warn("sources config missing themes array; using empty theme list");
    return { themes: [] };
  }

  const themes = value.themes.filter((theme, index) => {
    const valid = Boolean(theme && typeof theme.theme === "string" && theme.theme.trim());
    if (!valid) warn("skipping invalid theme config", { index });
    return valid;
  });

  info("sources config normalized", { themes: themes.length, skipped: value.themes.length - themes.length });
  return { themes };
}

function normalizeProcessed(value: ProcessedInformation[]): ProcessedInformation[] {
  if (!Array.isArray(value)) {
    warn("processed information file is not an array; using empty item list");
    return [];
  }

  info("processed information normalized", { items: value.length });
  return value;
}

async function main(): Promise<void> {
  configureLogging();
  info("site population started", { repo: repoRoot, processed_path: processedPath, sources_path: sourcesPath, index_path: indexPath });
  const sources = normalizeSources(readJson<SourcesConfig>(sourcesPath));
  const processed = normalizeProcessed(readJson<ProcessedInformation[]>(processedPath));
  const data = await buildData(sources, processed);
  updateIndex(data);
  info("site population completed", { home_articles: data.home.length, themes: data.themes.length });
}

main()
  .catch((err: Error) => {
    errorLog("site population failed", { error: err.stack || err.message });
    process.exitCode = 1;
  })
  .finally(async () => {
    await closeLogging();
    if (process.exitCode) process.exit(process.exitCode);
  });
