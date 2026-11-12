// Package browser opens URLs in the user's browser, including from WSL
// where the browser lives on the Windows side.
package browser

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	pkgbrowser "github.com/pkg/browser"
)

func init() {
	// pkg/browser writes the launcher's output to our stdout by default,
	// which would corrupt the TUI.
	pkgbrowser.Stdout, pkgbrowser.Stderr = nil, nil
}

// IsWSL reports whether we are running under Windows Subsystem for Linux.
func IsWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	b, err := os.ReadFile("/proc/version")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
}

// Open launches url in a browser. It does not wait for the browser to exit.
func Open(url string) error {
	if os.Getenv("REZKIT_NO_BROWSER") != "" {
		return errors.New("browser launch disabled by REZKIT_NO_BROWSER")
	}
	if IsWSL() {
		if p, err := exec.LookPath("wslview"); err == nil {
			return exec.Command(p, url).Start()
		}
		// rundll32 takes the URL as a single argument, avoiding cmd.exe's
		// special handling of characters like '&'.
		if p, err := exec.LookPath("rundll32.exe"); err == nil {
			return exec.Command(p, "url.dll,FileProtocolHandler", url).Start()
		}
	}
	return pkgbrowser.OpenURL(url)
}
