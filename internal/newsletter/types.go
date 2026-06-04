package newsletter

import "encoding/json"

type SourcesConfig struct {
	Themes []ThemeConfig `json:"themes"`
}

type ThemeConfig struct {
	Theme   string         `json:"theme"`
	Sources []SourceConfig `json:"sources"`
}

type SourceConfig struct {
	Name            string          `json:"name"`
	URL             string          `json:"url"`
	Type            string          `json:"type"`
	Tier            json.RawMessage `json:"tier,omitempty"`
	Auth            string          `json:"auth,omitempty"`
	FallbackURL     string          `json:"fallback_url,omitempty"`
	InsecureSSL     bool            `json:"insecure_ssl,omitempty"`
	IncludeURLRegex string          `json:"include_url_regex,omitempty"`
	ExcludeURLRegex string          `json:"exclude_url_regex,omitempty"`
	MaxItems        int             `json:"max_items,omitempty"`
	Raw             map[string]any  `json:"-"`
}

type Source struct {
	Theme  string          `json:"theme"`
	Name   string          `json:"name"`
	URL    string          `json:"url"`
	Type   string          `json:"type"`
	Tier   json.RawMessage `json:"tier,omitempty"`
	Config SourceConfig    `json:"config"`
}

type Collection struct {
	SourcePath  string   `json:"source_path"`
	ThemeCount  int      `json:"theme_count"`
	SourceCount int      `json:"source_count"`
	Sources     []Source `json:"sources"`
}

type Processed struct {
	ContentType string `json:"content_type"`
	Bytes       int    `json:"bytes"`
	Data        any    `json:"data"`
}

type ProcessedSource struct {
	Theme      string          `json:"theme"`
	Name       string          `json:"name"`
	URL        string          `json:"url"`
	Type       string          `json:"type"`
	Config     SourceConfig    `json:"config"`
	Processed  *Processed      `json:"processed,omitempty"`
	OK         bool            `json:"ok"`
	Error      string          `json:"error,omitempty"`
	ParseError string          `json:"parse_error,omitempty"`
	Info       []Information   `json:"information,omitempty"`
	Tier       json.RawMessage `json:"-"`
}

type Information struct {
	URL           string `json:"url"`
	Title         string `json:"title"`
	DatePublished string `json:"date_published"`
	Description   string `json:"description"`
}

type PrintableItem struct {
	Index       int    `json:"index"`
	Source      string `json:"source"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}
