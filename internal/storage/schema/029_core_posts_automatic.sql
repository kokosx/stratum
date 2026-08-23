-- 029_core_posts_automatic.sql
-- Upgrade core/posts to support source=automatic (preferred) with archive as alias.
UPDATE block_definitions
SET
    schema_json = '{"schemaVersion":1,"props":{"type":"object","properties":{}},"settings":{"type":"object","properties":{"source":{"type":"string","enum":["automatic","archive","latest"],"default":"automatic"},"layout":{"type":"string","enum":["list","grid"],"default":"list"},"columns":{"type":"integer","enum":[1,2,3],"default":1},"showImage":{"type":"boolean","default":true},"showDate":{"type":"boolean","default":true},"showExcerpt":{"type":"boolean","default":true},"limit":{"type":"integer","default":3,"minimum":1,"maximum":20},"pagination":{"type":"boolean","default":true},"showViewAll":{"type":"boolean","default":false},"viewAllLabel":{"type":"string","default":""}}},"children":{"mode":"none"},"editor":{"category":"query","icon":"posts","fields":{"settings.source":{"label":"Source","control":"select","group":"Content"},"settings.layout":{"label":"Layout","control":"segmented","group":"Style"},"settings.columns":{"label":"Columns","control":"select","group":"Style"},"settings.showImage":{"label":"Show image","control":"checkbox","group":"Content"},"settings.showDate":{"label":"Show date","control":"checkbox","group":"Content"},"settings.showExcerpt":{"label":"Show excerpt","control":"checkbox","group":"Content"},"settings.limit":{"label":"Number of posts","control":"number","group":"Content"},"settings.pagination":{"label":"Show pagination","control":"checkbox","group":"Content"},"settings.showViewAll":{"label":"Show view all link","control":"checkbox","group":"Content"},"settings.viewAllLabel":{"label":"View all label","control":"text","group":"Content"}}}}',
    template = '{{ $raw := .Settings.source }}{{ $isAuto := or (eq $raw "automatic") (eq $raw "archive") (eq $raw "") }}
{{ if $isAuto }}
  {{ if .Context.Archive }}
    {{ $entries := .Context.Archive.Entries }}{{ $settings := .Settings }}
    {{ if eq (len $entries) 0 }}
      <div class="stratum-posts-empty"><p>No posts yet.</p></div>
    {{ else }}
      <section class="stratum-posts stratum-posts--{{ $settings.layout }}{{ if eq $settings.layout "grid" }} stratum-posts--cols-{{ $settings.columns }}{{ end }}">
        {{ range $entries }}
          <article class="stratum-post-card">
            {{ if $settings.showImage }}{{ if .FeaturedImage.Src }}<figure class="stratum-post-card__media"><img src="{{ .FeaturedImage.Src }}"{{ if .FeaturedImage.SrcSet }} srcset="{{ .FeaturedImage.SrcSet }}" sizes="{{ if eq $settings.layout "grid" }}{{ if eq $settings.columns 2 }}(min-width: 768px) 50vw, 100vw{{ else if eq $settings.columns 3 }}(min-width: 768px) 33vw, 100vw{{ else }}(min-width: 768px) min(720px, 100vw), 100vw{{ end }}{{ else }}(min-width: 768px) min(720px, 100vw), 100vw{{ end }}"{{ end }}{{ if .FeaturedImage.Width }} width="{{ .FeaturedImage.Width }}"{{ end }}{{ if .FeaturedImage.Height }} height="{{ .FeaturedImage.Height }}"{{ end }} alt="{{ .FeaturedImage.Alt }}" loading="lazy" decoding="async"></figure>{{ end }}{{ end }}
            <header class="stratum-post-card__header"><h2 class="stratum-post-card__title"><a href="{{ .URL }}">{{ .Title }}</a></h2>{{ if $settings.showDate }}{{ if .PublishedISO }}<time class="stratum-post-card__date" datetime="{{ .PublishedISO }}">{{ .PublishedAt }}</time>{{ end }}{{ end }}</header>
            {{ if $settings.showExcerpt }}{{ if .Excerpt }}<p class="stratum-post-card__excerpt">{{ .Excerpt }}</p>{{ end }}{{ end }}
          </article>
        {{ end }}
      </section>
      {{ if $settings.pagination }}{{ $p := $.Context.Archive.Pagination }}{{ if gt $p.TotalPages 1 }}<nav aria-label="Pagination" class="stratum-pagination">{{ if $p.PreviousURL }}<a href="{{ $p.PreviousURL }}" rel="prev">Previous</a>{{ end }}<span>Page {{ $p.Current }} of {{ $p.TotalPages }}</span>{{ if $p.NextURL }}<a href="{{ $p.NextURL }}" rel="next">Next</a>{{ end }}</nav>{{ end }}{{ end }}
    {{ end }}
  {{ else }}
    {{ $entries := index .Context.Collections .ID }}{{ $settings := .Settings }}
    {{ if not $entries }}
      {{ if .Context.IsPreview }}<div class="stratum-posts-placeholder">Posts will appear here.</div>{{ else }}<div class="stratum-posts-empty"><p>No posts yet.</p></div>{{ end }}
    {{ else }}
      <section class="stratum-posts stratum-posts--{{ $settings.layout }}{{ if eq $settings.layout "grid" }} stratum-posts--cols-{{ $settings.columns }}{{ end }}">
        {{ range $entries }}
          <article class="stratum-post-card">
            {{ if $settings.showImage }}{{ if .FeaturedImage.Src }}<figure class="stratum-post-card__media"><img src="{{ .FeaturedImage.Src }}"{{ if .FeaturedImage.SrcSet }} srcset="{{ .FeaturedImage.SrcSet }}" sizes="{{ if eq $settings.layout "grid" }}{{ if eq $settings.columns 2 }}(min-width: 768px) 50vw, 100vw{{ else if eq $settings.columns 3 }}(min-width: 768px) 33vw, 100vw{{ else }}(min-width: 768px) min(720px, 100vw), 100vw{{ end }}{{ else }}(min-width: 768px) min(720px, 100vw), 100vw{{ end }}"{{ end }}{{ if .FeaturedImage.Width }} width="{{ .FeaturedImage.Width }}"{{ end }}{{ if .FeaturedImage.Height }} height="{{ .FeaturedImage.Height }}"{{ end }} alt="{{ .FeaturedImage.Alt }}" loading="lazy" decoding="async"></figure>{{ end }}{{ end }}
            <header class="stratum-post-card__header"><h2 class="stratum-post-card__title"><a href="{{ .URL }}">{{ .Title }}</a></h2>{{ if $settings.showDate }}{{ if .PublishedISO }}<time class="stratum-post-card__date" datetime="{{ .PublishedISO }}">{{ .PublishedAt }}</time>{{ end }}{{ end }}</header>
            {{ if $settings.showExcerpt }}{{ if .Excerpt }}<p class="stratum-post-card__excerpt">{{ .Excerpt }}</p>{{ end }}{{ end }}
          </article>
        {{ end }}
      </section>
      {{ if $settings.showViewAll }}{{ if $.Context.ArchiveURL }}<p class="stratum-posts-view-all"><a href="{{ $.Context.ArchiveURL }}">{{ if $settings.viewAllLabel }}{{ $settings.viewAllLabel }}{{ else }}View all posts{{ end }}</a></p>{{ end }}{{ end }}
    {{ end }}
  {{ end }}
{{ else }}
  {{ $entries := index .Context.Collections .ID }}{{ $settings := .Settings }}
  {{ if not $entries }}
    {{ if .Context.IsPreview }}<div class="stratum-posts-placeholder">Posts will appear here.</div>{{ else }}<div class="stratum-posts-empty"><p>No posts yet.</p></div>{{ end }}
  {{ else }}
    <section class="stratum-posts stratum-posts--{{ $settings.layout }}{{ if eq $settings.layout "grid" }} stratum-posts--cols-{{ $settings.columns }}{{ end }}">
      {{ range $entries }}
        <article class="stratum-post-card">
          {{ if $settings.showImage }}{{ if .FeaturedImage.Src }}<figure class="stratum-post-card__media"><img src="{{ .FeaturedImage.Src }}"{{ if .FeaturedImage.SrcSet }} srcset="{{ .FeaturedImage.SrcSet }}" sizes="{{ if eq $settings.layout "grid" }}{{ if eq $settings.columns 2 }}(min-width: 768px) 50vw, 100vw{{ else if eq $settings.columns 3 }}(min-width: 768px) 33vw, 100vw{{ else }}(min-width: 768px) min(720px, 100vw), 100vw{{ end }}{{ else }}(min-width: 768px) min(720px, 100vw), 100vw{{ end }}"{{ end }}{{ if .FeaturedImage.Width }} width="{{ .FeaturedImage.Width }}"{{ end }}{{ if .FeaturedImage.Height }} height="{{ .FeaturedImage.Height }}"{{ end }} alt="{{ .FeaturedImage.Alt }}" loading="lazy" decoding="async"></figure>{{ end }}{{ end }}
          <header class="stratum-post-card__header"><h2 class="stratum-post-card__title"><a href="{{ .URL }}">{{ .Title }}</a></h2>{{ if $settings.showDate }}{{ if .PublishedISO }}<time class="stratum-post-card__date" datetime="{{ .PublishedISO }}">{{ .PublishedAt }}</time>{{ end }}{{ end }}</header>
          {{ if $settings.showExcerpt }}{{ if .Excerpt }}<p class="stratum-post-card__excerpt">{{ .Excerpt }}</p>{{ end }}{{ end }}
        </article>
      {{ end }}
    </section>
  {{ end }}
  {{ if $settings.showViewAll }}{{ if $.Context.ArchiveURL }}<p class="stratum-posts-view-all"><a href="{{ $.Context.ArchiveURL }}">{{ if $settings.viewAllLabel }}{{ $settings.viewAllLabel }}{{ else }}View all posts{{ end }}</a></p>{{ end }}{{ end }}
{{ end }}
',
    updated_at = unixepoch()
WHERE id = 'core-posts-v1';
