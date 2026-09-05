// Package opds implements the OPDS 1.x/Atom subset used by Calibre-Web.
package opds

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
)

const (
	AcquisitionRel = "http://opds-spec.org/acquisition"
	EPUBMediaType  = "application/epub+zip"
	maxPages       = 100
	maxResponse    = 100 << 20
)

var (
	ErrNoEPUB            = errors.New("opds: entry has no EPUB acquisition link")
	ErrSearchUnavailable = errors.New("opds: catalog does not advertise search")
)

type Auth struct {
	Username, Password string
	// Origin limits requests and redirects to one catalog origin. Public client
	// operations anchor an empty Origin to their initial catalog URL.
	Origin string
}
type Link struct {
	Rel, Href, Type, Title string
}
type Entry struct {
	ID, Title string
	Links     []Link
}
type Feed struct {
	Title   string
	Entries []Entry
	Links   []Link
}

type atomLink struct {
	Rel   string `xml:"rel,attr"`
	Href  string `xml:"href,attr"`
	Type  string `xml:"type,attr"`
	Title string `xml:"title,attr"`
}
type atomEntry struct {
	ID    string     `xml:"id"`
	Title string     `xml:"title"`
	Links []atomLink `xml:"link"`
}
type atomFeed struct {
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
	Links   []atomLink  `xml:"link"`
}

type Client struct {
	HTTP *http.Client
	Auth Auth
}

func NewClient(httpClient *http.Client, auth Auth) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{HTTP: httpClient, Auth: auth}
}

func (c *Client) ListRoot(ctx context.Context, catalogURL string) (Feed, error) {
	return c.List(ctx, catalogURL)
}

// ListLanguages returns the Calibre-Web language navigation feed.
func (c *Client) ListLanguages(ctx context.Context, catalogURL string) (Feed, error) {
	return c.List(ctx, catalogEndpoint(catalogURL, "language"))
}

// ListLanguage returns every page of books in a Calibre-Web language feed.
func (c *Client) ListLanguage(ctx context.Context, catalogURL, languageID string) (Feed, error) {
	if id, err := strconv.Atoi(languageID); err != nil || id < 1 {
		return Feed{}, errors.New("opds: invalid Calibre-Web language id")
	}
	return c.List(ctx, catalogEndpoint(catalogURL, "language", languageID))
}

// List follows rel=next links and returns one combined feed. Callers that
// render pagination should use ListPage so one request maps to one feed page.
func (c *Client) List(ctx context.Context, feedURL string) (Feed, error) {
	scoped, err := c.withOrigin(feedURL)
	if err != nil {
		return Feed{}, err
	}
	if scoped != c {
		return scoped.List(ctx, feedURL)
	}
	var combined Feed
	seen := map[string]bool{}
	for page := 0; feedURL != ""; page++ {
		if page == maxPages {
			return Feed{}, errors.New("opds: pagination exceeds 100 pages")
		}
		if seen[feedURL] {
			return Feed{}, errors.New("opds: pagination cycle detected")
		}
		seen[feedURL] = true
		feed, err := c.ListPage(ctx, feedURL)
		if err != nil {
			return Feed{}, err
		}
		if combined.Title == "" {
			combined.Title = feed.Title
			combined.Links = feed.Links
		}
		combined.Entries = append(combined.Entries, feed.Entries...)
		feedURL = linkByRel(feed.Links, "next")
	}
	return combined, nil
}

// ListPage fetches exactly one feed page and preserves that page's navigation
// links for callers that expose pagination to a user.
func (c *Client) ListPage(ctx context.Context, feedURL string) (Feed, error) {
	if strings.TrimSpace(feedURL) == "" {
		return Feed{}, errors.New("opds: feed URL is required")
	}
	scoped, err := c.withOrigin(feedURL)
	if err != nil {
		return Feed{}, err
	}
	if scoped != c {
		return scoped.ListPage(ctx, feedURL)
	}
	return c.fetchFeed(ctx, feedURL)
}

func (c *Client) Search(ctx context.Context, catalogURL, query string) (Feed, error) {
	scoped, err := c.withOrigin(catalogURL)
	if err != nil {
		return Feed{}, err
	}
	if scoped != c {
		return scoped.Search(ctx, catalogURL, query)
	}
	target, err := c.searchTarget(ctx, catalogURL, query)
	if err != nil {
		return Feed{}, err
	}
	return c.List(ctx, target)
}

// SearchPage resolves the catalog's search endpoint and fetches only its first
// page. A caller can subsequently use ListPage for rel=next/previous links.
func (c *Client) SearchPage(ctx context.Context, catalogURL, query string) (Feed, error) {
	scoped, err := c.withOrigin(catalogURL)
	if err != nil {
		return Feed{}, err
	}
	if scoped != c {
		return scoped.SearchPage(ctx, catalogURL, query)
	}
	target, err := c.searchTarget(ctx, catalogURL, query)
	if err != nil {
		return Feed{}, err
	}
	return c.ListPage(ctx, target)
}

func (c *Client) searchTarget(ctx context.Context, catalogURL, query string) (string, error) {
	root, err := c.fetchFeed(ctx, catalogURL)
	if err != nil {
		return "", err
	}
	search := findLink(root.Links, "search")
	if search == nil {
		return "", ErrSearchUnavailable
	}
	template := search.Href
	if strings.Contains(strings.ToLower(search.Type), "opensearchdescription") {
		template, err = c.fetchSearchTemplate(ctx, search.Href)
		if err != nil {
			return "", err
		}
	}
	if !strings.Contains(template, "{searchTerms}") {
		return "", errors.New("opds: search template has no {searchTerms} placeholder")
	}
	return strings.ReplaceAll(template, "{searchTerms}", url.QueryEscape(query)), nil
}

func FindEPUBs(entry Entry) []Link {
	var out []Link
	for _, link := range entry.Links {
		acquisition := link.Rel == AcquisitionRel || strings.HasPrefix(link.Rel, AcquisitionRel+"/")
		if acquisition && strings.EqualFold(strings.TrimSpace(strings.Split(link.Type, ";")[0]), EPUBMediaType) {
			out = append(out, link)
		}
	}
	return out
}

// FilterEPUBEntries removes catalog entries that do not offer an EPUB.
// Calibre-Web's language endpoint filters by language but not by file format.
func FilterEPUBEntries(feed Feed) Feed {
	entries := make([]Entry, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		if len(FindEPUBs(entry)) > 0 {
			entries = append(entries, entry)
		}
	}
	feed.Entries = entries
	return feed
}

// LanguageID returns the Calibre-Web numeric language identifier advertised by
// a language navigation feed. It shares the same link conventions as the web
// catalog browser and never accepts an identifier that is not present in the
// owner-scoped feed.
func LanguageID(feed Feed, language, displayName string) string {
	normalizedLanguage := canonicalization.NormalizeLanguage(language)
	if normalizedLanguage != "" {
		for _, entry := range feed.Entries {
			if canonicalization.NormalizeLanguage(strings.TrimSpace(entry.Title)) != normalizedLanguage {
				continue
			}
			if id := languageEntryID(entry); id != "" {
				return id
			}
		}
	}

	for _, entry := range feed.Entries {
		name := strings.TrimSpace(entry.Title)
		if !sameLanguageLabel(name, language) && !strings.EqualFold(name, strings.TrimSpace(displayName)) {
			continue
		}
		if id := languageEntryID(entry); id != "" {
			return id
		}
	}
	return ""
}

func languageEntryID(entry Entry) string {
	for _, link := range entry.Links {
		if link.Rel != "subsection" && link.Rel != "alternate" && !(strings.EqualFold(link.Type, "application/atom+xml") && len(FindEPUBs(entry)) == 0) {
			continue
		}
		parsed, err := url.Parse(link.Href)
		if err != nil {
			continue
		}
		id, err := strconv.Atoi(strings.Trim(strings.TrimRight(parsed.Path, "/"), "/"))
		if err != nil || id < 1 {
			parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
			if len(parts) == 0 {
				continue
			}
			id, err = strconv.Atoi(parts[len(parts)-1])
			if err != nil || id < 1 {
				continue
			}
		}
		return strconv.Itoa(id)
	}
	return ""
}

func sameLanguageLabel(left, right string) bool {
	left = canonicalization.NormalizeLanguage(left)
	right = canonicalization.NormalizeLanguage(right)
	if left == "" || right == "" {
		return left == right
	}
	if left == right {
		return true
	}
	leftBase, rightBase := left, right
	if index := strings.IndexByte(leftBase, '-'); index >= 0 {
		leftBase = leftBase[:index]
	}
	if index := strings.IndexByte(rightBase, '-'); index >= 0 {
		rightBase = rightBase[:index]
	}
	return leftBase == rightBase
}

// SupportsSearch reports whether a feed advertises an OpenSearch endpoint.
func SupportsSearch(feed Feed) bool { return findLink(feed.Links, "search") != nil }

func (c *Client) Download(ctx context.Context, downloadURL string) ([]byte, error) {
	scoped, err := c.withOrigin(downloadURL)
	if err != nil {
		return nil, fmt.Errorf("opds: download EPUB: %w", err)
	}
	if scoped != c {
		return scoped.Download(ctx, downloadURL)
	}
	resp, err := c.get(ctx, downloadURL)
	if err != nil {
		return nil, fmt.Errorf("opds: download EPUB: %w", err)
	}
	defer resp.Body.Close()
	if err := statusError(resp); err != nil {
		return nil, err
	}
	mediaType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if mediaType != "" && !strings.EqualFold(mediaType, EPUBMediaType) && !strings.EqualFold(mediaType, "application/octet-stream") {
		return nil, fmt.Errorf("opds: download returned incompatible media type %q", mediaType)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return nil, fmt.Errorf("opds: read EPUB: %w", err)
	}
	if len(data) > maxResponse {
		return nil, errors.New("opds: EPUB exceeds 100 MiB limit")
	}
	return data, nil
}

func (c *Client) withOrigin(target string) (*Client, error) {
	if c.Auth.Origin != "" {
		return c, nil
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.User != nil || !validHTTPURL(parsed) {
		return nil, errors.New("opds: URL contains unsupported credentials or scheme")
	}
	scoped := *c
	scoped.Auth.Origin = parsed.Scheme + "://" + parsed.Host
	return &scoped, nil
}

func (c *Client) fetchFeed(ctx context.Context, feedURL string) (Feed, error) {
	resp, err := c.get(ctx, feedURL)
	if err != nil {
		return Feed{}, fmt.Errorf("opds: fetch feed: %w", err)
	}
	defer resp.Body.Close()
	if err := statusError(resp); err != nil {
		return Feed{}, err
	}
	var raw atomFeed
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&raw); err != nil {
		return Feed{}, fmt.Errorf("opds: parse Atom feed: %w", err)
	}
	base := resp.Request.URL
	feed := Feed{Title: strings.TrimSpace(raw.Title), Links: resolveLinks(base, raw.Links)}
	for _, item := range raw.Entries {
		feed.Entries = append(feed.Entries, Entry{ID: strings.TrimSpace(item.ID), Title: strings.TrimSpace(item.Title), Links: resolveLinks(base, item.Links)})
	}
	return feed, nil
}

func (c *Client) fetchSearchTemplate(ctx context.Context, descriptionURL string) (string, error) {
	resp, err := c.get(ctx, descriptionURL)
	if err != nil {
		return "", fmt.Errorf("opds: fetch search description: %w", err)
	}
	defer resp.Body.Close()
	if err := statusError(resp); err != nil {
		return "", err
	}
	var description struct {
		URLs []struct {
			Type     string `xml:"type,attr"`
			Template string `xml:"template,attr"`
		} `xml:"Url"`
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&description); err != nil {
		return "", fmt.Errorf("opds: parse search description: %w", err)
	}
	for _, candidate := range description.URLs {
		if strings.EqualFold(candidate.Type, "application/atom+xml") && strings.Contains(candidate.Template, "{searchTerms}") {
			template := resolveURL(resp.Request.URL, candidate.Template)
			if err := c.validateTarget(template); err != nil {
				return "", err
			}
			return template, nil
		}
	}
	return "", errors.New("opds: search description has no Atom template")
}

func (c *Client) get(ctx context.Context, target string) (*http.Response, error) {
	if err := c.validateTarget(target); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/atom+xml, application/xml;q=0.9, "+EPUBMediaType+";q=0.8")
	if (c.Auth.Username != "" || c.Auth.Password != "") && (c.Auth.Origin == "" || sameOrigin(c.Auth.Origin, target)) {
		req.SetBasicAuth(c.Auth.Username, c.Auth.Password)
	}
	return c.httpClient().Do(req)
}

func (c *Client) validateTarget(target string) error {
	parsed, err := url.Parse(target)
	if err != nil || parsed.User != nil || !validHTTPURL(parsed) {
		return errors.New("opds: URL contains unsupported credentials or scheme")
	}
	if c.Auth.Origin != "" {
		origin, err := url.Parse(c.Auth.Origin)
		if err != nil || origin.User != nil || !validHTTPURL(origin) || !sameOriginURL(origin, parsed) {
			return errors.New("opds: URL is outside the catalog origin")
		}
	}
	return nil
}

func (c *Client) httpClient() *http.Client {
	client := *c.HTTP
	customRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if customRedirect != nil {
			if err := customRedirect(req, via); err != nil {
				return err
			}
		}
		if err := c.validateTarget(req.URL.String()); err != nil {
			// Do not allow a custom redirect policy to turn an unsafe redirect
			// into a request carrying the catalog credentials.
			req.Header.Del("Authorization")
			return http.ErrUseLastResponse
		}
		return nil
	}
	return &client
}

func sameOrigin(left, right string) bool {
	a, errA := url.Parse(left)
	b, errB := url.Parse(right)
	return errA == nil && errB == nil && sameOriginURL(a, b)
}

func sameOriginURL(left, right *url.URL) bool {
	return left != nil && right != nil && validHTTPURL(left) && validHTTPURL(right) && strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}

func validHTTPURL(target *url.URL) bool {
	return target != nil && (strings.EqualFold(target.Scheme, "http") || strings.EqualFold(target.Scheme, "https")) && target.Host != ""
}
func statusError(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("opds: HTTP %s", resp.Status)
}
func resolveLinks(base *url.URL, links []atomLink) []Link {
	out := make([]Link, 0, len(links))
	for _, item := range links {
		out = append(out, Link{Rel: item.Rel, Href: resolveURL(base, item.Href), Type: item.Type, Title: item.Title})
	}
	return out
}
func catalogEndpoint(catalogURL string, segments ...string) string {
	parsed, err := url.Parse(catalogURL)
	if err != nil {
		return catalogURL
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + strings.Join(segments, "/")
	parsed.RawPath = ""
	parsed.Fragment = ""
	return parsed.String()
}
func resolveURL(base *url.URL, href string) string {
	reference, err := url.Parse(href)
	if err != nil {
		return href
	}
	resolved := base.ResolveReference(reference)
	if base.RawQuery == "" || reference.IsAbs() || reference.Host != "" {
		return resolved.String()
	}

	// Query parameters on the catalog URL can carry credentials. ResolveReference
	// replaces the base query when a relative link has its own query, or drops it
	// when the link only changes the path, so inherit base parameters that the
	// relative link does not override.
	baseQuery := base.Query()
	resolvedQuery := resolved.Query()
	for key, values := range baseQuery {
		if _, exists := resolvedQuery[key]; !exists {
			resolvedQuery[key] = values
		}
	}
	resolved.RawQuery = resolvedQuery.Encode()
	return resolved.String()
}
func findLink(links []Link, rel string) *Link {
	for i := range links {
		for _, value := range strings.Fields(links[i].Rel) {
			if value == rel {
				return &links[i]
			}
		}
	}
	return nil
}
func linkByRel(links []Link, rel string) string {
	if link := findLink(links, rel); link != nil {
		return link.Href
	}
	return ""
}
