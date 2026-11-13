package screens

import (
	"os"
	"testing"

	"github.com/rezkit/terminal/internal/api"
)

func TestMain(m *testing.M) {
	api.SetLocale("en-GB") // money rendering must not depend on the machine's locale
	os.Exit(m.Run())
}
