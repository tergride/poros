package engine

import (
	"net/http"
	"testing"
)

func chosenServer(t *testing.T, e *Engine) string {
	t.Helper()
	settings, err := e.Settings()
	if err != nil {
		t.Fatal(err)
	}
	return settings.Server
}

func choose(t *testing.T, e *Engine, id string) {
	t.Helper()
	settings, _ := e.Settings()
	settings.Server = id
	if _, err := e.SetSettings(settings); err != nil {
		t.Fatalf("choose %q: %v", id, err)
	}
}

func TestChosenServerFollowsWhatGoes(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	first, second := newProvider(t), newProvider(t)
	first.serve(http.StatusOK, vless("de.example.com", "DE"), vless("fi.example.com", "FI"))
	second.serve(http.StatusOK, vless("nl.example.com", "NL"))
	a, _ := e.AddSubscription(t.Context(), AddSubscription{URL: first.url})
	b, _ := e.AddSubscription(t.Context(), AddSubscription{URL: second.url})
	inA, _ := e.Servers(ServersQuery{GroupID: a.Group.ID})
	inB, _ := e.Servers(ServersQuery{GroupID: b.Group.ID})

	settings, _ := e.Settings()
	settings.Server = "missing"
	if _, err := e.SetSettings(settings); reason(err) != ReasonNotFound {
		t.Fatalf("a server that is not there must be refused: %v", err)
	}

	// An update that drops the chosen server moves the choice to the first one left in that subscription.
	choose(t, e, inA[1].ID)
	first.serve(http.StatusOK, vless("de.example.com", "DE"))
	if _, err := e.UpdateSubscription(t.Context(), a.Group.ID); err != nil {
		t.Fatal(err)
	}
	if got := chosenServer(t, e); got != inA[0].ID {
		t.Fatalf("after the update the choice is %q, not the server left (%q)", got, inA[0].ID)
	}
	// Removing the subscription moves it to the first server of another, and removing that one clears it.
	if err := e.RemoveGroup(a.Group.ID); err != nil {
		t.Fatal(err)
	}
	if got := chosenServer(t, e); got != inB[0].ID {
		t.Fatalf("after the removal the choice is %q, not %q", got, inB[0].ID)
	}
	if err := e.RemoveGroup(b.Group.ID); err != nil {
		t.Fatal(err)
	}
	if got := chosenServer(t, e); got != "" {
		t.Fatalf("with no servers left nothing is chosen, not %q", got)
	}
}

func TestPresetNamesStayApart(t *testing.T) {
	e, _ := openTest(t, t.TempDir())
	draft := PresetCopy{BasedOn: "all", Name: "Games"}
	games, err := e.SavePreset(draft)
	if err != nil {
		t.Fatal(err)
	}
	draft.Name = " games "
	if _, err = e.SavePreset(draft); reason(err) != ReasonPresetNameTaken {
		t.Fatalf("a second preset of the same name: %v", err)
	}
	draft.Name = "All traffic"
	if _, err = e.SavePreset(draft); reason(err) != ReasonPresetNameTaken {
		t.Fatalf("the name of a built-in preset: %v", err)
	}
	// A preset keeps its own name when it is saved again.
	own, _ := e.PresetCopy(games.ID)
	if _, err = e.SavePreset(own); err != nil {
		t.Fatalf("saving a preset under its own name: %v", err)
	}
	link, _ := e.ExportPreset(games.ID)
	imported, err := e.ImportPreset(link)
	if err != nil || imported.Name != "Games 2" {
		t.Fatalf("an imported preset of a taken name gets a number: %+v, %v", imported, err)
	}
}
