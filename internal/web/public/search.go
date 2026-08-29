package public

import (
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/themes"
)

func (h *Handler) serveSearch(w http.ResponseWriter, r *http.Request) {
	if h.search == nil {
		http.Error(w, "Search unavailable", http.StatusServiceUnavailable)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	typeFilter := strings.TrimSpace(r.URL.Query().Get("type"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	// Use filtered query; service validates typeFilter internally.
	results, total, counts, err := h.search.QueryFiltered(r.Context(), query, typeFilter, page)
	if err != nil {
		log.Printf("search query failed: %v", err)
		http.Error(w, "Search is temporarily unavailable.", http.StatusInternalServerError)
		return
	}
	validatedFilter := h.search.ValidateFilter(r.Context(), typeFilter)

	var contentBuilder strings.Builder
	contentBuilder.WriteString(`<main class="stratum-search">`)
	contentBuilder.WriteString(`<h1>Search</h1>`)
	contentBuilder.WriteString(`<form method="get" action="/search" role="search">`)
	contentBuilder.WriteString(`<label for="search-input">Search</label> `)
	contentBuilder.WriteString(`<input id="search-input" type="search" name="q" value="`)
	contentBuilder.WriteString(template.HTMLEscapeString(query))
	contentBuilder.WriteString(`" placeholder="">`)
	if typeFilter != "" {
		// Preserve type filter when searching again from paginated/filtered view via hidden? No, new search should reset to All.
		// But we keep query preservation via GET; type is not preserved on new submit unless user clicks filter.
		// Do not add hidden type input to avoid confusing UX; user explicitly filters via nav.
	}
	contentBuilder.WriteString(` <button type="submit">Search</button>`)
	contentBuilder.WriteString(`</form>`)

	if query == "" {
		// Empty query: just show form, no header or results.
		contentBuilder.WriteString(`</main>`)
		renderSearchPage(w, r, h, query, contentBuilder.String())
		return
	}

	// Header with count
	if total == 1 {
		contentBuilder.WriteString(`<p>1 result for “` + template.HTMLEscapeString(query) + `”</p>`)
	} else if total == 0 {
		contentBuilder.WriteString(`<p>No results for “` + template.HTMLEscapeString(query) + `”.</p><p>Try a different search.</p>`)
	} else {
		contentBuilder.WriteString(`<p>` + strconv.Itoa(total) + ` results for “` + template.HTMLEscapeString(query) + `”</p>`)
	}

	// Filter navigation if multiple types or filter active
	if len(counts) > 0 && query != "" {
		// Build sorted keys for stable output
		keys := make([]string, 0, len(counts))
		for k := range counts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		// Show filter nav if we have at least 2 types or a filter is active
		if len(keys) > 1 || validatedFilter != "" {
			contentBuilder.WriteString(`<nav aria-label="Filter by content type">`)
			// All link
			allCount := totalUnfiltered(counts)
			// When filtered, All count is totalUnfiltered, current page total is filtered total.
			// Use counts map for All.
			contentBuilder.WriteString(buildFilterLink(query, "", validatedFilter == "", "All", allCount))
			for _, k := range keys {
				label := h.searchTypeLabel(r, k)
				cnt := counts[k]
				contentBuilder.WriteString(buildFilterLink(query, k, validatedFilter == k, label, cnt))
			}
			contentBuilder.WriteString(`</nav>`)
		}
	}

	if total > 0 && len(results) > 0 {
		for _, result := range results {
			contentBuilder.WriteString(`<article class="search-result">`)
			contentBuilder.WriteString(`<h2><a href="` + template.HTMLEscapeString(result.Path) + `">` + template.HTMLEscapeString(result.Title) + `</a></h2>`)
			if result.ContentTypeLabel != "" {
				contentBuilder.WriteString(`<p class="search-meta">` + template.HTMLEscapeString(result.ContentTypeLabel) + `</p>`)
			}
			snippet := strings.TrimSpace(result.Snippet)
			if snippet == "" {
				snippet = template.HTMLEscapeString(result.Excerpt)
			}
			if snippet != "" {
				// snippet already contains escaped fragments with <mark>, safe to embed as raw HTML
				contentBuilder.WriteString(`<p class="search-snippet">` + snippet + `</p>`)
			}
			contentBuilder.WriteString(`</article>`)
		}
		// Pagination
		totalPages := (total + 10 - 1) / 10
		if totalPages > 1 {
			contentBuilder.WriteString(`<nav aria-label="Search results pages">`)
			if page > 1 {
				contentBuilder.WriteString(`<a href="` + template.HTMLEscapeString(buildSearchURL(query, validatedFilter, page-1)) + `">Previous</a> `)
			}
			for p := 1; p <= totalPages && p <= 10; p++ {
				// Show all pages up to 10; for larger, we limit display but still functional
				// Simple: show 1..totalPages but capped to avoid huge nav
				if p == page {
					contentBuilder.WriteString(`<span aria-current="page">` + strconv.Itoa(p) + `</span> `)
				} else {
					contentBuilder.WriteString(`<a href="` + template.HTMLEscapeString(buildSearchURL(query, validatedFilter, p)) + `">` + strconv.Itoa(p) + `</a> `)
				}
			}
			if page < totalPages {
				contentBuilder.WriteString(`<a href="` + template.HTMLEscapeString(buildSearchURL(query, validatedFilter, page+1)) + `">Next</a>`)
			}
			contentBuilder.WriteString(`</nav>`)
		}
	} else if total == 0 {
		// Already rendered "No results" header, keep input populated (already done)
	}
	contentBuilder.WriteString(`</main>`)
	renderSearchPage(w, r, h, query, contentBuilder.String())
}

func renderSearchPage(w http.ResponseWriter, r *http.Request, h *Handler, query, innerHTML string) {
	siteSnap := h.hub.Site.Current()
	blocksCSS, themeCSS, themeJS := h.AssetURLs()
	title := "Search"
	if query != "" {
		title = `Search: ` + query
	}
	pageHTML, err := h.themes.Render(themes.PageView{
		Site:        themes.SiteView{Title: siteSnap.SiteTitle, Tagline: siteSnap.SiteTagline, Language: siteSnap.Language, SiteURL: siteSnap.SiteURL},
		Entry:       themes.EntryView{Title: "Search"},
		Head:        themes.HeadView{Title: title, Robots: "noindex, follow"},
		Navigation:  h.hub.Navigation.LocationsForPath("/search"),
		Content:     template.HTML(innerHTML),
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

func buildSearchURL(q, typeFilter string, page int) string {
	v := url.Values{}
	if q != "" {
		v.Set("q", q)
	}
	if typeFilter != "" {
		v.Set("type", typeFilter)
	}
	if page > 1 {
		v.Set("page", strconv.Itoa(page))
	}
	if len(v) == 0 {
		return "/search"
	}
	return "/search?" + v.Encode()
}

func buildFilterLink(q, typeVal string, isCurrent bool, label string, count int) string {
	href := buildSearchURL(q, typeVal, 1)
	escapedLabel := template.HTMLEscapeString(label)
	text := escapedLabel
	if count >= 0 {
		text = escapedLabel + " " + strconv.Itoa(count)
	}
	if isCurrent {
		return `<span aria-current="page">` + text + `</span> `
	}
	return `<a href="` + template.HTMLEscapeString(href) + `">` + text + `</a> `
}

func totalUnfiltered(counts map[string]int) int {
	sum := 0
	for _, c := range counts {
		sum += c
	}
	return sum
}

func (h *Handler) searchTypeLabel(r *http.Request, id string) string {
	// Try catalog first for accurate custom labels
	if h.queries != nil {
		if def, err := content.NewCatalog(h.queries).GetDefinition(r.Context(), id); err == nil {
			return def.Label()
		}
	}
	return content.DefinitionFor(id).Label()
}
