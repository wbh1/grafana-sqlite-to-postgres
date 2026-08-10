package sqlite

import (
	"io/ioutil"
	"path/filepath"
	"testing"
)

func TestSanitize_ConvertsIdentifierBackticks(t *testing.T) {
	tmp := t.TempDir()
	dumpFile := filepath.Join(tmp, "dump.sql")

	input := "INSERT INTO `user` VALUES(1, 'ok');\n"
	if err := ioutil.WriteFile(dumpFile, []byte(input), 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	if err := Sanitize(dumpFile); err != nil {
		t.Fatalf("sanitize: %v", err)
	}

	out, err := ioutil.ReadFile(dumpFile)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	want := "INSERT INTO \"user\" VALUES(1, 'ok');\n"
	if string(out) != want {
		t.Fatalf("unexpected output\nwant: %q\ngot:  %q", want, string(out))
	}
}

func TestSanitize_PreservesBackticksInsideStringValues(t *testing.T) {
	tmp := t.TempDir()
	dumpFile := filepath.Join(tmp, "dump.sql")

	input := "INSERT INTO `dashboard` VALUES(1, 'title with `backticks` and ''quote''');\n"
	if err := ioutil.WriteFile(dumpFile, []byte(input), 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	if err := Sanitize(dumpFile); err != nil {
		t.Fatalf("sanitize: %v", err)
	}

	out, err := ioutil.ReadFile(dumpFile)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	want := "INSERT INTO \"dashboard\" VALUES(1, 'title with `backticks` and ''quote''');\n"
	if string(out) != want {
		t.Fatalf("unexpected output\nwant: %q\ngot:  %q", want, string(out))
	}
}
