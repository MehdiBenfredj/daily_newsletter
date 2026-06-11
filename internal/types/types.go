package types

type SourcesConfig struct {
	Themes []ThemeConfig `json:"themes"`
}

type ThemeConfig struct {
	Theme   string         `json:"theme"`
	Sources []SourceConfig `json:"sources"`
}

type SourceConfig struct {
	Name            string         `json:"name"`
	URL             string         `json:"url"`
	Type            string         `json:"type"`
	Tier            int            `json:"tier,omitempty"`
	Auth            string         `json:"auth,omitempty"`
	FallbackURL     string         `json:"fallback_url,omitempty"`
	InsecureSSL     bool           `json:"insecure_ssl,omitempty"`
	IncludeURLRegex string         `json:"include_url_regex,omitempty"`
	ExcludeURLRegex string         `json:"exclude_url_regex,omitempty"`
	MaxItems        int            `json:"max_items,omitempty"`
	Raw             map[string]any `json:"-"`
}

type Source struct {
	Theme  string       `json:"theme"`
	Name   string       `json:"name"`
	URL    string       `json:"url"`
	Type   string       `json:"type"`
	Tier   int          `json:"tier,omitempty"`
	Config SourceConfig `json:"config"`
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
	Theme      string        `json:"theme"`
	Name       string        `json:"name"`
	URL        string        `json:"url"`
	Type       string        `json:"type"`
	Config     SourceConfig  `json:"config"`
	Processed  *Processed    `json:"processed,omitempty"`
	OK         bool          `json:"ok"`
	Error      string        `json:"error,omitempty"`
	ParseError string        `json:"parse_error,omitempty"`
	Info       []Information `json:"information,omitempty"`
	Tier       int           `json:"tier,omitempty"`
}

type Information struct {
	URL           string `json:"url"`
	Title         string `json:"title"`
	DatePublished string `json:"date_published"`
	Description   string `json:"description"`
}

type OutputItem struct {
	Index       int    `json:"index"`
	Source      string `json:"source"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Tier        int    `json:"tier,omitempty"`
	Theme       string `json:"theme,omitempty"`
	Rating      float64    `json:"rating"`
}

type OpenRouterChatRequest struct {
	Model       string              `json:"model"`
	Messages    []OpenRouterMessage `json:"messages"`
	Temperature float64             `json:"temperature"`
	MaxTokens   int                 `json:"max_tokens"`
}

type OpenRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenRouterChatResponse struct {
	Choices []struct {
		Message OpenRouterMessage `json:"message"`
	} `json:"choices"`
}
