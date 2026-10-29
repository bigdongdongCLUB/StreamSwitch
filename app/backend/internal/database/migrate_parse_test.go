package database

import "testing"

func TestSchemaParse(t *testing.T) {
	stmts := splitSQL(schemaSQL)
	tables, cols := 0, 0
	for _, s := range stmts {
		m := createTableRe.FindStringSubmatch(s)
		if m == nil {
			t.Fatalf("unexpected stmt: %.80s", s)
		}
		tables++
		for _, line := range splitLines(m[2]) {
			if cm := columnLineRe.FindStringSubmatch(line); cm != nil {
				cols++
				if cm[2][len(cm[2])-1] == ',' {
					t.Fatalf("trailing comma kept: %s", line)
				}
			}
		}
	}
	if tables != 19 || cols < 150 {
		t.Fatalf("tables=%d cols=%d", tables, cols)
	}
	if n := len(splitSQL(seedAlwaysSQL)); n != 5 {
		t.Fatalf("seed_always stmts=%d", n)
	}
	if n := len(splitSQL(seedInitSQL)); n != 2 {
		t.Fatalf("seed_init stmts=%d", n)
	}
	t.Logf("tables=%d cols=%d", tables, cols)
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
