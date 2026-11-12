package api

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	SetLocale("en_GB.UTF-8") // tests must not depend on the machine's locale
	os.Exit(m.Run())
}

func TestFormatMoney(t *testing.T) {
	defer SetLocale("en-GB")
	cases := []struct {
		locale string
		m      Money
		want   string
	}{
		{"en-GB", Money{0, "GBP"}, "£0.00"},
		{"en-GB", Money{123456789, "GBP"}, "£1,234,567.89"},
		{"en-GB", Money{-5, "GBP"}, "-£0.05"},
		{"en_US.UTF-8", Money{123456, "USD"}, "$1,234.56"},
		{"de_DE.UTF-8@euro", Money{123456, "EUR"}, "1.234,56\u00a0€"},
		{"de-DE", Money{123456, "GBP"}, "1.234,56\u00a0£"},
		{"en-GB", Money{123456, "JPY"}, "JP¥123,456"},       // no minor unit
		{"en-GB", Money{123456, "BHD"}, "BHD\u00a0123.456"}, // three decimals
		{"C", Money{123456, "GBP"}, "£1,234.56"},
		{"en-GB", Money{123456, "XQQ"}, "XQQ 1234.56"}, // unknown code
	}
	for _, c := range cases {
		SetLocale(c.locale)
		if got := FormatMoney(c.m); got != c.want {
			t.Errorf("%s %+v = %q, want %q", c.locale, c.m, got, c.want)
		}
	}
}

func TestLocaleFromEnv(t *testing.T) {
	t.Setenv("REZKIT_LOCALE", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MONETARY", "fr_FR.UTF-8")
	t.Setenv("LANG", "en_US.UTF-8")
	if got := normaliseLocale(LocaleFromEnv()); got != "fr-FR" {
		t.Errorf("LC_MONETARY should beat LANG, got %q", got)
	}
	t.Setenv("REZKIT_LOCALE", "de-DE")
	if got := normaliseLocale(LocaleFromEnv()); got != "de-DE" {
		t.Errorf("REZKIT_LOCALE should win, got %q", got)
	}
}
