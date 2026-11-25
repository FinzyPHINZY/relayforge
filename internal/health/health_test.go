package health

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// SchemaVersion must name the newest migration, or readiness passes on a database
// that is missing the latest schema.
func TestSchemaVersionMatchesNewestMigration(t *testing.T) {
	paths, err := filepath.Glob("../../db/migrations/*.up.sql")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no migrations found: %v", err)
	}
	highest := 0
	for _, p := range paths {
		prefix, _, ok := strings.Cut(filepath.Base(p), "_")
		if !ok {
			t.Fatalf("migration %s has no numeric prefix", p)
		}
		n, err := strconv.Atoi(prefix)
		if err != nil {
			t.Fatalf("migration %s: %v", p, err)
		}
		highest = max(highest, n)
	}
	if SchemaVersion != highest {
		t.Fatalf("SchemaVersion=%d, newest migration is %d", SchemaVersion, highest)
	}
}
