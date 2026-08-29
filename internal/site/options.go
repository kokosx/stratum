package site

type LanguageOption struct {
	Value string
	Label string
}

type TimezoneOption struct {
	Value string
	Label string
}

// LanguageOptions returns the full supported language list (same as Settings UI).
func LanguageOptions() []LanguageOption {
	return []LanguageOption{
		{Value: "en", Label: "English"},
		{Value: "en-US", Label: "English (United States)"},
		{Value: "en-GB", Label: "English (United Kingdom)"},
		{Value: "pl", Label: "Polski"},
		{Value: "de", Label: "Deutsch"},
		{Value: "fr", Label: "Français"},
		{Value: "es", Label: "Español"},
		{Value: "it", Label: "Italiano"},
		{Value: "pt", Label: "Português"},
		{Value: "nl", Label: "Nederlands"},
		{Value: "ja", Label: "日本語"},
		{Value: "zh", Label: "中文"},
		{Value: "ru", Label: "Русский"},
		{Value: "uk", Label: "Українська"},
		{Value: "cs", Label: "Čeština"},
	}
}

// CreatorLanguageOptions returns the small Creator catalog (English + Polski only).
// Normal Settings supports the full list above.
func CreatorLanguageOptions() []LanguageOption {
	return []LanguageOption{
		{Value: "en", Label: "English"},
		{Value: "pl", Label: "Polski"},
	}
}

// TimezoneOptions returns the canonical timezone choices.
func TimezoneOptions() []TimezoneOption {
	return []TimezoneOption{
		{Value: "UTC", Label: "UTC — Coordinated Universal Time"},
		{Value: "Europe/Warsaw", Label: "Europe/Warsaw — Warsaw"},
		{Value: "Europe/Berlin", Label: "Europe/Berlin — Berlin"},
		{Value: "Europe/Paris", Label: "Europe/Paris — Paris"},
		{Value: "Europe/London", Label: "Europe/London — London"},
		{Value: "Europe/Rome", Label: "Europe/Rome — Rome"},
		{Value: "Europe/Madrid", Label: "Europe/Madrid — Madrid"},
		{Value: "Europe/Prague", Label: "Europe/Prague — Prague"},
		{Value: "America/New_York", Label: "America/New_York — New York"},
		{Value: "America/Chicago", Label: "America/Chicago — Chicago"},
		{Value: "America/Los_Angeles", Label: "America/Los_Angeles — Los Angeles"},
		{Value: "America/Sao_Paulo", Label: "America/Sao_Paulo — São Paulo"},
		{Value: "Asia/Tokyo", Label: "Asia/Tokyo — Tokyo"},
		{Value: "Asia/Shanghai", Label: "Asia/Shanghai — Shanghai"},
		{Value: "Asia/Dubai", Label: "Asia/Dubai — Dubai"},
		{Value: "Australia/Sydney", Label: "Australia/Sydney — Sydney"},
	}
}

func IsLanguageInOptions(val string, opts []LanguageOption) bool {
	for _, o := range opts {
		if o.Value == val {
			return true
		}
	}
	return false
}

func IsTimezoneInOptions(val string, opts []TimezoneOption) bool {
	for _, o := range opts {
		if o.Value == val {
			return true
		}
	}
	return false
}

func IsValidCreatorLanguage(val string) bool {
	return val == "en" || val == "pl"
}
