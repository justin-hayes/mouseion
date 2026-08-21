// Package opds implements the OPDS 1.x/Atom subset used by Calibre-Web.
package opds

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	AcquisitionRel = "http://opds-spec.org/acquisition"
	EPUBMediaType  = "application/epub+zip"
	maxPages       = 100
	maxResponse    = 100 << 20
)

var ErrNoEPUB = errors.New("opds: entry has no EPUB acquisition link")

type Auth struct {
	Username, Password string
	// Origin limits where credentials are sent. Empty preserves compatibility
	// for callers that intentionally authenticate to arbitrary URLs.
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

// List follows rel=next links and returns one combined feed.
func (c *Client) List(ctx context.Context, feedURL string) (Feed, error) {
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
		feed, err := c.fetchFeed(ctx, feedURL)
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

func (c *Client) Search(ctx context.Context, catalogURL, query string) (Feed, error) {
	root, err := c.fetchFeed(ctx, catalogURL)
	if err != nil {
		return Feed{}, err
	}
	search := findLink(root.Links, "search")
	if search == nil {
		return Feed{}, errors.New("opds: catalog does not advertise search")
	}
	template := search.Href
	if strings.Contains(strings.ToLower(search.Type), "opensearchdescription") {
		template, err = c.fetchSearchTemplate(ctx, search.Href)
		if err != nil {
			return Feed{}, err
		}
	}
	if !strings.Contains(template, "{searchTerms}") {
		return Feed{}, errors.New("opds: search template has no {searchTerms} placeholder")
	}
	return c.List(ctx, strings.ReplaceAll(template, "{searchTerms}", url.QueryEscape(query)))
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

func (c *Client) Download(ctx context.Context, downloadURL string) ([]byte, error) {
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

func (c *Client) fetchFeed(ctx context.Context, feedURL string) (Feed, error) {
	resp, err := c.get(ctx, feedURL)
	if err != nil {
		return Feed{}, fmt.Errorf("opds: fetch feed: %w", err)
	}
	defer resp.Body.Close()
	if err := statusError(resp); err != nil {
		return Feed{}, err
	}
	// Read a bounded prefix so we can (a) detect HTML responses that are not
	// OPDS feeds and (b) still feed the full body to the XML decoder.
	prefix, err := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if err != nil {
		return Feed{}, fmt.Errorf("opds: read feed: %w", err)
	}
	body := prefix
	if len(body) == 16<<10 { // not truncated in practice; append reader rest if present
		rest, err := io.ReadAll(resp.Body)
		if err != nil {
			return Feed{}, fmt.Errorf("opds: read feed: %w", err)
		}
		body = append(body, rest...)
	}
	if looksLikeHTML(body) {
		return Feed{}, errors.New("opds: server returned an HTML page, not an OPDS Atom feed — check the catalog URL and that it points at the OPDS endpoint (e.g. /opds), and that authentication is configured")
	}
	var raw atomFeed
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(&raw); err != nil {
		return Feed{}, fmt.Errorf("opds: parse Atom feed: %w", err)
	}
	base := resp.Request.URL
	feed := Feed{Title: strings.TrimSpace(raw.Title), Links: resolveLinks(base, raw.Links)}
	for _, item := range raw.Entries {
		feed.Entries = append(feed.Entries, Entry{ID: strings.TrimSpace(item.ID), Title: strings.TrimSpace(item.Title), Links: resolveLinks(base, item.Links)})
	}
	return feed, nil
}

// looksLikeHTML reports whether the bytes look like an HTML document rather
// than an Atom/XML feed. Calibre-Web and other servers return an HTML login or
// wrapper page for some OPDS URLs; feeding that to the XML decoder yields an
// unhelpful "element <link> closed by </head>" error.
func looksLikeHTML(body []byte) bool {
	head := strings.ToLower(string(body[:min(len(body), 2048)]))
	if strings.Contains(head, "<!doctype html") || strings.Contains(head, "<html") || strings.Contains(head, "<head>") {
		return true
	}
	// XHTML served with an HTML root (e.g. a login page) also trips the parser.
	return strings.Contains(head, "<body") && !strings.Contains(head, "<feed")
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
			return resolveURL(resp.Request.URL, candidate.Template), nil
		}
	}
	return "", errors.New("opds: search description has no Atom template")
}

func (c *Client) get(ctx context.Context, target string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/atom+xml, application/xml;q=0.9, "+EPUBMediaType+";q=0.8")
	if (c.Auth.Username != "" || c.Auth.Password != "") && (c.Auth.Origin == "" || sameOrigin(c.Auth.Origin, target)) {
		req.SetBasicAuth(c.Auth.Username, c.Auth.Password)
	}
	return c.HTTP.Do(req)
}
func sameOrigin(left, right string) bool {
	a, errA := url.Parse(left)
	b, errB := url.Parse(right)
	return errA == nil && errB == nil && strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
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
func resolveURL(base *url.URL, href string) string {
	reference, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(reference).String()
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
