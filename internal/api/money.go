package api

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/bojanz/currency"
)

// Money formatting follows the user's locale (see SetLocale / LocaleFromEnv).
var (
	moneyMu  sync.RWMutex
	moneyFmt = currency.NewFormatter(currency.NewLocale("en"))
)

// SetLocale sets the locale used by FormatMoney, as a BCP 47 tag ("en-GB")
// or a POSIX locale name ("de_DE.UTF-8"). It returns the normalised tag.
func SetLocale(name string) string {
	tag := normaliseLocale(name)
	moneyMu.Lock()
	moneyFmt = currency.NewFormatter(currency.NewLocale(tag))
	moneyMu.Unlock()
	return tag
}

// LocaleFromEnv returns the locale for monetary values: REZKIT_LOCALE, then
// the POSIX precedence LC_ALL, LC_MONETARY, LANG.
func LocaleFromEnv() string {
	for _, v := range []string{"REZKIT_LOCALE", "LC_ALL", "LC_MONETARY", "LANG"} {
		if s := os.Getenv(v); s != "" {
			return s
		}
	}
	return ""
}

// normaliseLocale turns "en_GB.UTF-8@euro" into "en-GB"; C, POSIX and empty
// become "en".
func normaliseLocale(s string) string {
	if i := strings.IndexAny(s, ".@"); i >= 0 {
		s = s[:i]
	}
	s = strings.ReplaceAll(s, "_", "-")
	if s == "" || s == "C" || s == "POSIX" {
		return "en"
	}
	return s
}

// FormatMoney renders an amount in the currency's smallest unit (pence,
// cents, yen...) for the current locale, e.g. "£1,234.56" or "1.234,56 €".
func FormatMoney(m Money) string {
	amt, err := currency.NewAmountFromInt64(m.Minor, m.Currency)
	if err != nil { // unknown currency code: assume two decimals
		sign, v := "", m.Minor
		if v < 0 {
			sign, v = "-", -v
		}
		return fmt.Sprintf("%s%s %d.%02d", sign, m.Currency, v/100, v%100)
	}
	moneyMu.RLock()
	defer moneyMu.RUnlock()
	return moneyFmt.Format(amt)
}
