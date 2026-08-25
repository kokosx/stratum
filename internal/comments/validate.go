package comments

import (
	"net/mail"
	"net/url"
	"strings"
)

func validateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrInvalidName
	}
	if len(name) > MaxNameLength {
		return ErrInvalidName
	}
	return nil
}

func validateEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" || len(email) > MaxEmailLength {
		return ErrInvalidEmail
	}
	_, err := mail.ParseAddress(email)
	if err != nil {
		return ErrInvalidEmail
	}
	if !strings.Contains(email, "@") {
		return ErrInvalidEmail
	}
	return nil
}

func validateURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if len(raw) > MaxURLLength {
		return ErrInvalidURL
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ErrInvalidURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrInvalidURL
	}
	if u.Host == "" {
		return ErrInvalidURL
	}
	return nil
}

func validateBody(body string) error {
	body = normalizeBody(body)
	if body == "" {
		return ErrBodyRequired
	}
	if len(body) < MinBodyLength {
		return ErrBodyRequired
	}
	if len(body) > MaxBodyLength {
		return ErrBodyTooLong
	}
	return nil
}
