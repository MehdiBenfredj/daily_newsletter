package parse

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	stdhtml "html"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/MehdiBenfredj/daily_newsletter/internal/types"
)

var whitespace = regexp.MustCompile(`\s+`)

type xmlNode struct {
	Name     string
	Attrs    map[string]string
	Text     strings.Builder
	Children []*xmlNode
}

func WasPublishedInLast24Hours(value string, now time.Time) bool {
	published, ok := ParsePublishedDatetime(value)
	if !ok {
		return false
	}
	now = now.UTC()
	return !published.Before(now.Add(-24*time.Hour)) && !published.After(now)
}

func ParsePublishedDatetime(value string) (time.Time, bool) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return time.Time{}, false
	}

	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func ProcessedSource(source types.ProcessedSource, now time.Time) ([]types.Information, error) {
	if source.Processed == nil {
		return nil, nil
	}
	switch strings.ToLower(defaultType(source.Type)) {
	case "rss", "feed", "atom", "xml":
		items, err := rssInformation(source.Processed)
		if err != nil {
			return nil, err
		}
		return filterRecent(items, now), nil
	case "website", "html", "web":
		return websiteInformation(source)
	case "api":
		items, err := apiInformation(source)
		if err != nil {
			return nil, err
		}
		return filterRecent(items, now), nil
	default:
		return nil, fmt.Errorf("unsupported source type %q for %s", source.Type, source.Name)
	}
}

func defaultType(value string) string {
	if value == "" {
		return "rss"
	}
	return value
}

func filterRecent(items []types.Information, now time.Time) []types.Information {
	recent := make([]types.Information, 0, len(items))
	for _, item := range items {
		if WasPublishedInLast24Hours(item.DatePublished, now) {
			recent = append(recent, item)
		}
	}
	return recent
}

func rssInformation(processed *types.Processed) ([]types.Information, error) {
	raw, ok := processed.Data.(string)
	if !ok {
		return nil, nil
	}
	root, err := parseXML(raw)
	if err != nil {
		return nil, err
	}

	var entries []*xmlNode
	if channel := firstChild(root, "channel"); channel != nil {
		entries = children(channel, "item")
	}
	if len(entries) == 0 {
		entries = descendants(root, "entry")
	}

	items := make([]types.Information, 0, len(entries))
	for _, entry := range entries {
		link := textOf(entry, "link")
		if link == "" {
			if linkNode := firstChild(entry, "link"); linkNode != nil {
				link = linkNode.Attrs["href"]
			}
		}
		items = append(items, types.Information{
			URL:           link,
			Title:         textOf(entry, "title"),
			DatePublished: textOf(entry, "pubDate", "published", "updated", "date"),
			Description:   textOf(entry, "description", "summary"),
		})
	}
	return items, nil
}

func parseXML(raw string) (*xmlNode, error) {
	decoder := xml.NewDecoder(strings.NewReader(raw))
	var stack []*xmlNode
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		switch token := token.(type) {
		case xml.StartElement:
			node := &xmlNode{Name: strings.ToLower(token.Name.Local), Attrs: map[string]string{}}
			for _, attr := range token.Attr {
				node.Attrs[strings.ToLower(attr.Name.Local)] = attr.Value
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, node)
			}
			stack = append(stack, node)
		case xml.CharData:
			if len(stack) > 0 {
				_, _ = stack[len(stack)-1].Text.Write(token)
			}
		case xml.EndElement:
			if len(stack) == 1 {
				return stack[0], nil
			}
			stack = stack[:len(stack)-1]
		}
	}
	return nil, fmt.Errorf("empty XML document")
}

func firstChild(node *xmlNode, names ...string) *xmlNode {
	wanted := wantedNames(names)
	for _, child := range node.Children {
		if wanted[child.Name] {
			return child
		}
	}
	return nil
}

func children(node *xmlNode, name string) []*xmlNode {
	var result []*xmlNode
	for _, child := range node.Children {
		if child.Name == strings.ToLower(name) {
			result = append(result, child)
		}
	}
	return result
}

func descendants(node *xmlNode, name string) []*xmlNode {
	var result []*xmlNode
	if node.Name == strings.ToLower(name) {
		result = append(result, node)
	}
	for _, child := range node.Children {
		result = append(result, descendants(child, name)...)
	}
	return result
}

func textOf(node *xmlNode, names ...string) string {
	child := firstChild(node, names...)
	if child == nil {
		return ""
	}
	return cleanText(child.Text.String())
}

func wantedNames(names []string) map[string]bool {
	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[strings.ToLower(name)] = true
	}
	return wanted
}

func websiteInformation(source types.ProcessedSource) ([]types.Information, error) {
	raw, ok := source.Processed.Data.(string)
	if !ok {
		return nil, nil
	}
	base, err := url.Parse(source.URL)
	if err != nil {
		return nil, err
	}
	include, err := compileOptional(source.Config.IncludeURLRegex)
	if err != nil {
		return nil, err
	}
	exclude, err := compileOptional(source.Config.ExcludeURLRegex)
	if err != nil {
		return nil, err
	}
	maxItems := source.Config.MaxItems
	if maxItems <= 0 {
		maxItems = 25
	}

	root, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var items []types.Information
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && strings.EqualFold(node.Data, "a") {
			if len(items) >= maxItems {
				return
			}
			item := linkInformation(base, node)
			if item.URL == "" || item.URL == source.URL || seen[item.URL] || len(item.Title) < 8 {
				return
			}
			if include != nil && !include.MatchString(item.URL) {
				return
			}
			if exclude != nil && exclude.MatchString(item.URL) {
				return
			}
			seen[item.URL] = true
			items = append(items, item)
		}
		for child := node.FirstChild; child != nil && len(items) < maxItems; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return items, nil
}

func compileOptional(pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, nil
	}
	return regexp.Compile(pattern)
}

func linkInformation(base *url.URL, node *html.Node) types.Information {
	var href string
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, "href") {
			href = stdhtml.UnescapeString(attr.Val)
			break
		}
	}
	parsed, err := url.Parse(href)
	if err != nil {
		return types.Information{}
	}
	resolved := base.ResolveReference(parsed)
	resolved.Fragment = ""
	return types.Information{
		URL:   resolved.String(),
		Title: cleanText(nodeText(node)),
	}
}

func nodeText(node *html.Node) string {
	var buf bytes.Buffer
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			buf.WriteString(current.Data)
			buf.WriteByte(' ')
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return buf.String()
}

func apiInformation(source types.ProcessedSource) ([]types.Information, error) {
	data, ok := source.Processed.Data.(map[string]any)
	if !ok {
		return nil, nil
	}
	linesByID := map[string]string{}
	if lines, ok := data["lines"].([]any); ok {
		for _, value := range lines {
			line, ok := value.(map[string]any)
			if !ok {
				continue
			}
			id := stringValue(line["id"])
			name := stringValue(line["shortName"])
			if name == "" {
				name = stringValue(line["name"])
			}
			label := strings.TrimSpace(strings.TrimSpace(stringValue(line["mode"])) + " " + name)
			if id != "" {
				linesByID[id] = label
			}
		}
	}

	disruptions, ok := data["disruptions"].([]any)
	if !ok {
		return nil, nil
	}
	items := make([]types.Information, 0, len(disruptions))
	for _, value := range disruptions {
		disruption, ok := value.(map[string]any)
		if !ok {
			continue
		}
		impactedLines := impactedLineNames(disruption, linesByID)
		details := []string{
			strings.Join(impactedLines, ", "),
			stringValue(disruption["severity"]),
			stringValue(disruption["cause"]),
			cleanTextWithoutHTML(firstNonEmpty(disruption["message"], disruption["shortMessage"])),
		}
		items = append(items, types.Information{
			URL:           source.URL,
			Title:         cleanTextWithoutHTML(firstNonEmpty(disruption["title"], disruption["shortMessage"])),
			DatePublished: stringValue(disruption["lastUpdate"]),
			Description:   joinNonEmpty(details, " | "),
		})
	}
	return items, nil
}

func impactedLineNames(disruption map[string]any, linesByID map[string]string) []string {
	var names []string
	seen := map[string]bool{}
	sections, ok := disruption["impactedSections"].([]any)
	if !ok {
		return names
	}
	for _, value := range sections {
		section, ok := value.(map[string]any)
		if !ok {
			continue
		}
		lineID := stringValue(section["lineId"])
		name := linesByID[lineID]
		if name == "" {
			name = lineID
		}
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func cleanTextWithoutHTML(value any) string {
	noTags := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(stringValue(value), " ")
	return cleanText(noTags)
}

func cleanText(value string) string {
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = stdhtml.UnescapeString(value)
	value = strings.ReplaceAll(value, "\u00a0", " ")
	return strings.TrimSpace(whitespace.ReplaceAllString(value, " "))
}

func firstNonEmpty(values ...any) any {
	for _, value := range values {
		if stringValue(value) != "" {
			return value
		}
	}
	return ""
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		encoded, _ := json.Marshal(typed)
		return string(encoded)
	}
}

func joinNonEmpty(values []string, sep string) string {
	var kept []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			kept = append(kept, value)
		}
	}
	return strings.Join(kept, sep)
}
