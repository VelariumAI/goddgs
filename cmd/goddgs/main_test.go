package main

import "testing"

func TestRunSearchValidationPaths(t *testing.T) {
	if code := runSearch([]string{"--badflag"}); code != 2 {
		t.Fatalf("code=%d want 2", code)
	}
	if code := runSearch([]string{"--q", "   "}); code != 2 {
		t.Fatalf("code=%d want 2", code)
	}
}

func TestRunProvidersAndMin(t *testing.T) {
	if code := runProviders(); code != 0 {
		t.Fatalf("runProviders code=%d want 0", code)
	}
	if min(1, 2) != 1 || min(2, 1) != 1 {
		t.Fatal("min function mismatch")
	}
}

func TestRunDispatchValidation(t *testing.T) {
	if code := run([]string{"goddgs"}); code != 2 {
		t.Fatalf("run no-subcommand code=%d want 2", code)
	}
	if code := run([]string{"goddgs", "unknown"}); code != 2 {
		t.Fatalf("run unknown code=%d want 2", code)
	}
}

func TestRunDispatchCommands(t *testing.T) {
	srv := newDDGTestServer()
	defer srv.Close()

	t.Setenv("GODDGS_DDG_BASE", srv.URL)
	t.Setenv("GODDGS_LINKS_BASE", srv.URL)
	t.Setenv("GODDGS_HTML_BASE", srv.URL)
	t.Setenv("GODDGS_TIMEOUT", "1s")

	if code := run([]string{"goddgs", "providers"}); code != 0 {
		t.Fatalf("providers code=%d want 0", code)
	}
	if code := run([]string{"goddgs", "search", "--q", "golang", "--json"}); code != 0 {
		t.Fatalf("search code=%d want 0", code)
	}
	if code := run([]string{"goddgs", "doctor"}); code != 0 {
		t.Fatalf("doctor code=%d want 0", code)
	}
}
