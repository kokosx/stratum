package creator

// creatorCopy provides EN/PL strings for structural starter copy.
// This is a small Creator-only catalog, not a CMS-wide i18n framework.
var creatorCopy = map[string]map[string]string{
	"en": {
		"nav.home":               "Home",
		"nav.blog":               "Blog",
		"nav.work":               "Work",
		"nav.products":           "Products",
		"nav.services":           "Services",
		"nav.about":              "About",
		"nav.contact":            "Contact",
		"heading.latest_posts":   "Latest posts",
		"heading.selected_work":  "Selected Work",
		"heading.testimonials":   "What clients say",
		"heading.featured":       "Featured Products",
		"heading.services":       "Services",
		"cta.start_conversation": "Start a conversation",
		"cta.request_consult":    "Request a consultation",
		"cta.contact_us":         "Contact us",
		"cta.next_step":          "Make the next step clear",
		"cta.need_next_step":     "Need a practical next step?",
		"lead.testimonial_next":  "Share what you are trying to achieve. We will respond with a practical next step.",
		"lead.local_next":        "Tell us what you need and we will explain how we can help.",
		"page.about.title":       "About",
		"page.about.body":        "Use this page to introduce your work, values and approach.",
		"page.contact.title":     "Contact",
		"form.project_enquiry":   "Project enquiry",
		"form.request_info":      "Request information",
		"form.contact":           "Contact",
	},
	"pl": {
		"nav.home":               "Strona główna",
		"nav.blog":               "Blog",
		"nav.work":               "Projekty",
		"nav.products":           "Produkty",
		"nav.services":           "Usługi",
		"nav.about":              "O nas",
		"nav.contact":            "Kontakt",
		"heading.latest_posts":   "Najnowsze wpisy",
		"heading.selected_work":  "Wybrane prace",
		"heading.testimonials":   "Co mówią klienci",
		"heading.featured":       "Polecane produkty",
		"heading.services":       "Usługi",
		"cta.start_conversation": "Rozpocznij rozmowę",
		"cta.request_consult":    "Poproś o konsultację",
		"cta.contact_us":         "Skontaktuj się",
		"cta.next_step":          "Zrób kolejny krok",
		"cta.need_next_step":     "Potrzebujesz praktycznego kolejnego kroku?",
		"lead.testimonial_next":  "Opowiedz, co chcesz osiągnąć. Odpowiemy praktycznym kolejnym krokiem.",
		"lead.local_next":        "Powiedz nam, czego potrzebujesz, a wyjaśnimy, jak możemy pomóc.",
		"page.about.title":       "O nas",
		"page.about.body":        "Wykorzystaj tę stronę, aby przedstawić swoją pracę, wartości i podejście.",
		"page.contact.title":     "Kontakt",
		"form.project_enquiry":   "Zapytanie o projekt",
		"form.request_info":      "Poproś o informacje",
		"form.contact":           "Kontakt",
	},
}

func copyFor(lang, key string) string {
	if m, ok := creatorCopy[lang]; ok {
		if v, ok := m[key]; ok && v != "" {
			return v
		}
	}
	if m, ok := creatorCopy["en"]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}

func localizedPageTitle(lang, slug string) string {
	switch slug {
	case "about":
		if lang == "pl" {
			return "O nas"
		}
		return "About"
	case "contact":
		if lang == "pl" {
			return "Kontakt"
		}
		return "Contact"
	default:
		return slug
	}
}

func localizedHeading(lang, key string) string { return copyFor(lang, key) }

// localizedProjectEntries returns project seed entries translated when lang==pl
func localizedProjectEntries(lang string) []entrySpec {
	if lang == "pl" {
		return []entrySpec{
			{Title: "Dom Północny", Slug: "dom-polnocny", Excerpt: "Zwięzły przykład projektu prezentujący założenia, proces i efekt.", Body: "Ta strona projektu jest gotowa na Twoją własną historię, zdjęcia i szczegóły.", Fields: map[string]any{"client": "Fikcyjny klient", "year": "2026", "services": "Kierunek, projekt"}},
			{Title: "Notatki Terenowe", Slug: "notatki-terenowe", Excerpt: "Zwięzły przykład projektu prezentujący założenia, proces i efekt.", Body: "Ta strona projektu jest gotowa na Twoją własną historię, zdjęcia i szczegóły.", Fields: map[string]any{"client": "Fikcyjny klient", "year": "2026", "services": "Kierunek, projekt"}},
			{Title: "Tożsamość Atlas", Slug: "tozsamosc-atlas", Excerpt: "Zwięzły przykład projektu prezentujący założenia, proces i efekt.", Body: "Ta strona projektu jest gotowa na Twoją własną historię, zdjęcia i szczegóły.", Fields: map[string]any{"client": "Fikcyjny klient", "year": "2026", "services": "Kierunek, projekt"}},
			{Title: "Studio Jeden", Slug: "studio-jeden", Excerpt: "Zwięzły przykład projektu prezentujący założenia, proces i efekt.", Body: "Ta strona projektu jest gotowa na Twoją własną historię, zdjęcia i szczegóły.", Fields: map[string]any{"client": "Fikcyjny klient", "year": "2026", "services": "Kierunek, projekt"}},
			{Title: "Wspólny Grunt", Slug: "wspolny-grunt", Excerpt: "Zwięzły przykład projektu prezentujący założenia, proces i efekt.", Body: "Ta strona projektu jest gotowa na Twoją własną historię, zdjęcia i szczegóły.", Fields: map[string]any{"client": "Fikcyjny klient", "year": "2026", "services": "Kierunek, projekt"}},
			{Title: "Sygnał", Slug: "sygnal", Excerpt: "Zwięzły przykład projektu prezentujący założenia, proces i efekt.", Body: "Ta strona projektu jest gotowa na Twoją własną historię, zdjęcia i szczegóły.", Fields: map[string]any{"client": "Fikcyjny klient", "year": "2026", "services": "Kierunek, projekt"}},
		}
	}
	return projectEntries()
}

func localizedProductEntries(lang string) []entrySpec {
	if lang == "pl" {
		return []entrySpec{
			{Title: "Krzesło Form", Slug: "krzeslo-form", Excerpt: "Przemyślany obiekt do codziennych przestrzeni.", Body: "Dodaj tutaj materiały, wymiary i inne szczegóły produktu.", Fields: map[string]any{"sku": "SAMPLE-A", "price_display": "1 020 zł", "short_description": "Prosty, trwały projekt do codziennego użytku."}},
			{Title: "Lampa Arc", Slug: "lampa-arc", Excerpt: "Przemyślany obiekt do codziennych przestrzeni.", Body: "Dodaj tutaj materiały, wymiary i inne szczegóły produktu.", Fields: map[string]any{"sku": "SAMPLE-B", "price_display": "760 zł", "short_description": "Prosty, trwały projekt do codziennego użytku."}},
			{Title: "Stół Studio", Slug: "stol-studio", Excerpt: "Przemyślany obiekt do codziennych przestrzeni.", Body: "Dodaj tutaj materiały, wymiary i inne szczegóły produktu.", Fields: map[string]any{"sku": "SAMPLE-C", "price_display": "od 2 700 zł", "short_description": "Prosty, trwały projekt do codziennego użytku."}},
			{Title: "Półka Field", Slug: "polka-field", Excerpt: "Przemyślany obiekt do codziennych przestrzeni.", Body: "Dodaj tutaj materiały, wymiary i inne szczegóły produktu.", Fields: map[string]any{"sku": "SAMPLE-D", "price_display": "1 350 zł", "short_description": "Prosty, trwały projekt do codziennego użytku."}},
			{Title: "Taca Mono", Slug: "taca-mono", Excerpt: "Przemyślany obiekt do codziennych przestrzeni.", Body: "Dodaj tutaj materiały, wymiary i inne szczegóły produktu.", Fields: map[string]any{"sku": "SAMPLE-E", "price_display": "320 zł", "short_description": "Prosty, trwały projekt do codziennego użytku."}},
			{Title: "Stołek Line", Slug: "stolek-line", Excerpt: "Przemyślany obiekt do codziennych przestrzeni.", Body: "Dodaj tutaj materiały, wymiary i inne szczegóły produktu.", Fields: map[string]any{"sku": "SAMPLE-F", "price_display": "890 zł", "short_description": "Prosty, trwały projekt do codziennego użytku."}},
		}
	}
	return productEntries()
}

func localizedBlogEntries(lang string) []entrySpec {
	if lang == "pl" {
		return []entrySpec{
			{Title: "Pierwsze kroki", Slug: "pierwsze-kroki", Excerpt: "Proste miejsce na początek.", Body: "Zacznij od podstaw, wprowadź jedną użyteczną zmianę i buduj dalej."},
			{Title: "Praktyczny przewodnik", Slug: "praktyczny-przewodnik", Excerpt: "Kilka przydatnych zasad codziennej pracy.", Body: "Jasne priorytety i małe, przemyślane kroki ułatwiają skomplikowaną pracę."},
			{Title: "Kulisy", Slug: "kulisy", Excerpt: "Spojrzenie na proces powstawania pracy.", Body: "Dobre rezultaty zwykle wymagają starannego przygotowania i cierpliwej korekty."},
			{Title: "Czego się nauczyliśmy", Slug: "czego-sie-nauczylismy", Excerpt: "Notatki z ostatniej pracy.", Body: "Najtrwalsze lekcje są często proste."},
			{Title: "Krótka aktualizacja", Slug: "krotka-aktualizacja", Excerpt: "Najnowsze wiadomości i co dalej.", Body: "Oto zwięzła aktualizacja dla czytelników."},
		}
	}
	return []entrySpec{
		{Title: "Getting started", Slug: "getting-started", Excerpt: "A straightforward place to begin.", Body: "Start with the essentials, make one useful change, and build from there."},
		{Title: "A practical guide", Slug: "a-practical-guide", Excerpt: "A few useful principles for everyday work.", Body: "Clear priorities and small, deliberate steps make complicated work easier to manage."},
		{Title: "Behind the scenes", Slug: "behind-the-scenes", Excerpt: "A look at the process behind the finished work.", Body: "Good results usually come from careful preparation, useful feedback and patient revision."},
		{Title: "What we've learned", Slug: "what-we-have-learned", Excerpt: "Notes from recent work.", Body: "The most durable lessons are often simple: listen closely, test assumptions and keep the result useful."},
		{Title: "A short update", Slug: "a-short-update", Excerpt: "Recent news and what comes next.", Body: "Here is a concise update for readers, with more details to follow soon."},
	}
}

func localizedServiceEntries(lang string) []entrySpec {
	if lang == "pl" {
		return []entrySpec{
			{Title: "Konsultacja", Slug: "konsultacja", Excerpt: "Skupiona rozmowa, aby zrozumieć pracę i zarekomendować kolejne kroki.", Body: "Przeglądamy potrzeby i proponujemy praktyczną drogę naprzód.", Fields: map[string]any{"short_summary": "Jasna porada i praktyczny kolejny krok.", "service_area": "Okolicę i zdalnie"}},
			{Title: "Instalacja", Slug: "instalacja", Excerpt: "Staranna konfiguracja z jasną komunikacją.", Body: "Instalacja planowana wokół Twojej przestrzeni i priorytetów.", Fields: map[string]any{"short_summary": "Staranna konfiguracja od początku do końca.", "service_area": "Okolicę"}},
			{Title: "Konserwacja", Slug: "konserwacja", Excerpt: "Regularna opieka.", Body: "Regularna konserwacja pomaga wcześnie wykryć drobne problemy.", Fields: map[string]any{"short_summary": "Regularna opieka dla niezawodnego działania.", "service_area": "Okolicę"}},
			{Title: "Wsparcie awaryjne", Slug: "wsparcie-awaryjne", Excerpt: "Szybka pomoc, gdy pilny problem wymaga uwagi.", Body: "Skontaktuj się, a wyjaśnimy dostępne opcje reakcji.", Fields: map[string]any{"short_summary": "Szybka pomoc w nagłych przypadkach.", "service_area": "Wybrane lokalne obszary"}},
			{Title: "Usługa niestandardowa", Slug: "usluga-niestandardowa", Excerpt: "Elastyczna opcja dla pracy nie mieszczącej się w pakiecie.", Body: "Możemy przygotować dostosowaną usługę po krótkiej rozmowie.", Fields: map[string]any{"short_summary": "Dostosowane podejście dla specyficznych wymagań.", "service_area": "Po uzgodnieniu"}},
		}
	}
	return []entrySpec{
		{Title: "Consultation", Slug: "consultation", Excerpt: "A focused conversation to understand the work and recommend next steps.", Body: "We review what you need, answer initial questions and outline a practical way forward.", Fields: map[string]any{"short_summary": "Clear advice and a practical next step.", "service_area": "Local area and remote"}},
		{Title: "Installation", Slug: "installation", Excerpt: "Careful setup with clear communication.", Body: "Installation is planned around your space, priorities and agreed schedule.", Fields: map[string]any{"short_summary": "Careful setup from start to finish.", "service_area": "Local area"}},
		{Title: "Maintenance", Slug: "maintenance", Excerpt: "Routine care that keeps things working well.", Body: "Regular maintenance helps identify small issues early and supports reliable day-to-day use.", Fields: map[string]any{"short_summary": "Routine care for reliable operation.", "service_area": "Local area"}},
		{Title: "Emergency support", Slug: "emergency-support", Excerpt: "Responsive help when an urgent issue needs attention.", Body: "Contact us with the details and we will explain the available response options.", Fields: map[string]any{"short_summary": "Responsive help for urgent issues.", "service_area": "Selected local areas"}},
		{Title: "Custom service", Slug: "custom-service", Excerpt: "A flexible option for work that does not fit a standard package.", Body: "We can scope a tailored service after a short conversation about your requirements.", Fields: map[string]any{"short_summary": "A tailored approach for specific requirements.", "service_area": "By arrangement"}},
	}
}

func localizedTestimonialEntries(lang string) []entrySpec {
	if lang == "pl" {
		return []entrySpec{
			{Title: "Maja Chen", Slug: "maja-chen", Fields: map[string]any{"quote": "Proces był jasny, przemyślany i łatwy do śledzenia.", "person": "Maja Chen", "role": "Lider operacyjny", "company": "Northline"}},
			{Title: "Samuel Rivera", Slug: "samuel-rivera", Fields: map[string]any{"quote": "Przeszliśmy od pomysłu do użytecznego rezultatu bez zbędnej złożoności.", "person": "Samuel Rivera", "role": "Założyciel", "company": "Common Field"}},
			{Title: "Aleks Morgan", Slug: "aleks-morgan", Fields: map[string]any{"quote": "Każda decyzja została wyjaśniona, a efekt końcowy jest nasz.", "person": "Aleks Morgan", "role": "Dyrektor", "company": "Studio Lane"}},
			{Title: "Jordan Lee", Slug: "jordan-lee", Fields: map[string]any{"quote": "Praktyczny partner od pierwszej rozmowy do startu.", "person": "Jordan Lee", "role": "Lider zespołu", "company": "Fieldwork"}},
		}
	}
	return []entrySpec{
		{Title: "Maya Chen", Slug: "maya-chen", Fields: map[string]any{"quote": "The process was clear, thoughtful and easy to follow.", "person": "Maya Chen", "role": "Operations lead", "company": "Northline"}},
		{Title: "Sam Rivera", Slug: "sam-rivera", Fields: map[string]any{"quote": "We moved from an idea to a useful result without unnecessary complexity.", "person": "Sam Rivera", "role": "Founder", "company": "Common Field"}},
		{Title: "Alex Morgan", Slug: "alex-morgan", Fields: map[string]any{"quote": "Every decision was explained and the finished work feels like ours.", "person": "Alex Morgan", "role": "Director", "company": "Studio Lane"}},
		{Title: "Jordan Lee", Slug: "jordan-lee", Fields: map[string]any{"quote": "A practical partner from the first conversation to launch.", "person": "Jordan Lee", "role": "Team lead", "company": "Fieldwork"}},
	}
}
