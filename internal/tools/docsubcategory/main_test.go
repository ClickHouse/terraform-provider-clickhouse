package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/registry"
)

func TestDocPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind     registry.Kind
		typeName string
		want     string
	}{
		{registry.KindResource, "clickhouse_service", filepath.Join("docs", "resources", "service.md")},
		{registry.KindDataSource, "clickhouse_clickstack_alert", filepath.Join("docs", "data-sources", "clickstack_alert.md")},
	}
	for _, c := range cases {
		if got := docPath(c.kind, c.typeName); got != c.want {
			t.Errorf("docPath(%v, %q) = %q, want %q", c.kind, c.typeName, got, c.want)
		}
	}
}

func TestStamp(t *testing.T) {
	t.Parallel()

	// A doc whose body also mentions subcategory: "" inline — the anchored match
	// must rewrite only the frontmatter line, never the body text.
	doc := "---\npage_title: x\n" + emptyFrontmatter + "\ndescription: |-\n" +
		"  set subcategory: \"\" to leave it blank\n---\n\n# x\n"

	p := filepath.Join(t.TempDir(), "x.md")
	if err := os.WriteFile(p, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := stamp(p, "ClickHouse Cloud"); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	got, _ := os.ReadFile(p)
	if want := "\nsubcategory: \"ClickHouse Cloud\"\n"; !strings.Contains(string(got), want) {
		t.Errorf("frontmatter not stamped:\n%s", got)
	}
	if want := "set subcategory: \"\" to leave it blank"; !strings.Contains(string(got), want) {
		t.Errorf("body line was clobbered:\n%s", got)
	}

	// Idempotent: a second stamp is a no-op.
	before, _ := os.ReadFile(p)
	if err := stamp(p, "ClickHouse Cloud"); err != nil {
		t.Fatalf("stamp (2nd): %v", err)
	}
	after, _ := os.ReadFile(p)
	if !reflect.DeepEqual(before, after) {
		t.Error("stamp is not idempotent")
	}

	// Error when there is no empty frontmatter line to stamp.
	np := filepath.Join(t.TempDir(), "n.md")
	if err := os.WriteFile(np, []byte("---\npage_title: x\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := stamp(np, "X"); err == nil {
		t.Error("expected error stamping a doc with no empty frontmatter")
	}
}

func TestUnstamped(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join("docs", "resources"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join("docs", "resources", name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("stamped.md", "---\n"+`subcategory: "X"`+"\n---\n")
	write("stray.md", "---\n"+emptyFrontmatter+"\n---\n")
	write("notdoc.txt", emptyFrontmatter) // ignored: not .md

	got, err := unstamped()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join("docs", "resources", "stray.md")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unstamped() = %v, want %v (missing data-sources dir must be skipped, not error)", got, want)
	}
}
