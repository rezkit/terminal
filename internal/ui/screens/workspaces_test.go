package screens

import (
	"context"
	"strings"
	"testing"

	"github.com/rezkit/terminal/internal/api"
)

// envFake reports workspaces with environments, which the shared fake lacks.
type envFake struct{ *api.Fake }

func (envFake) Workspaces(context.Context) ([]api.Workspace, error) {
	return []api.Workspace{
		{ID: "ws-1", Name: "Alpine Adventures", Environment: "Production"},
		{ID: "ws-2", Name: "Alpine Adventures", Environment: "Sandbox"},
		{ID: "ws-3", Name: "Coastal Cruises"},
	}, nil
}

func TestWorkspacePickerIsOneLinePerWorkspace(t *testing.T) {
	h := newHarness(t, envFake{api.NewFake()})
	want := []string{"Alpine Adventures · Production", "Alpine Adventures · Sandbox", "Coastal Cruises"}
	h.mustContain(want...)

	lines := strings.Split(stripANSI(h.view()), "\n")
	row := func(s string) int {
		for i, l := range lines {
			if strings.Contains(l, s) && (s != "Coastal Cruises" || !strings.Contains(l, "·")) {
				return i
			}
		}
		return -1
	}
	first := row(want[0])
	for i, s := range want {
		if got := row(s); got != first+i {
			t.Fatalf("%q on line %d, want %d (rows should be adjacent):\n%s", s, got, first+i, h.view())
		}
	}
}
