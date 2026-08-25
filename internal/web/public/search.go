package public

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/kokosx/stratum/internal/themes"
)

func (h *Handler) serveSearch(w http.ResponseWriter, r *http.Request) {
	if h.search == nil {
		http.Error(w, "Search unavailable", http.StatusServiceUnavailable)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	results, total, err := h.search.Query(r.Context(), query, page)
	if err != nil {
		http.Error(w, "Search unavailable", http.StatusInternalServerError)
		return
	}
	var content strings.Builder
	content.WriteString(`<main class="stratum-search-results"><h1>Search</h1><form method="get" action="/search"><label>Search <input type="search" name="q" value="`)
	content.WriteString(template.HTMLEscapeString(query))
	content.WriteString(`"></label><button type="submit">Search</button></form>`)
	if query != "" {
		content.WriteString(`<p>Results for “` + template.HTMLEscapeString(query) + `”</p>`)
		for _, result := range results {
			content.WriteString(`<article><h2><a href="` + template.HTMLEscapeString(result.Path) + `">` + template.HTMLEscapeString(result.Title) + `</a></h2><p>` + template.HTMLEscapeString(result.Excerpt) + `</p></article>`)
		}
		if total == 0 {
			content.WriteString(`<p>No results found.</p>`)
		}
		if page > 1 {
			content.WriteString(`<a href="/search?q=` + template.URLQueryEscaper(query) + `&page=` + strconv.Itoa(page-1) + `">Previous</a> `)
		}
		if page*10 < total {
			content.WriteString(`<a href="/search?q=` + template.URLQueryEscaper(query) + `&page=` + strconv.Itoa(page+1) + `">Next</a>`)
		}
	}
	content.WriteString(`</main>`)
	siteSnap := h.hub.Site.Current()
	blocksCSS, themeCSS, themeJS := h.AssetURLs()
	pageHTML, err := h.themes.Render(themes.PageView{
		Site:        themes.SiteView{Title: siteSnap.SiteTitle, Tagline: siteSnap.SiteTagline, Language: siteSnap.Language, SiteURL: siteSnap.SiteURL},
		Entry:       themes.EntryView{Title: "Search"},
		Head:        themes.HeadView{Title: "Search", Robots: "noindex, follow"},
		Navigation:  h.hub.Navigation.LocationsForPath("/search"),
		Content:     template.HTML(content.String()),
		ContentType: "page",
		Kind:        themes.PageKindSingle,
		Assets:      themes.AssetsView{BlocksCSS: blocksCSS, ThemeCSS: themeCSS, ThemeJS: themeJS},
	}, nil)
	if err != nil {
		http.Error(w, "Search unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex, follow")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodHead {
		_, _ = w.Write(pageHTML)
	}
}
