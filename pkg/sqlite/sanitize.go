package sqlite

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"regexp"
	"strings"
)

// Sanitize cleans up a SQLite dump file to prep it for import into Postgres.
func Sanitize(dumpFile string) error {
	data, err := ioutil.ReadFile(dumpFile)
	if err != nil {
		return err
	}
	// Convert SQLite identifier quotes (`name`) to Postgres style ("name"),
	// but preserve backticks that are part of string literal values.
	sanitized := normalizeBackticksOutsideStringLiterals(data)

	// Remove SQLite-specific PRAGMA statements
	// and statements that start with BEGIN
	// and statements pertaining to the sqlite_sequence table.
	re := regexp.MustCompile(`(?m)[\r\n]?^(PRAGMA.*;|BEGIN.*;|.*sqlite_sequence.*;)$`)
	sanitized = re.ReplaceAll(sanitized, nil)

	// Ensure there are quotes around table names to avoid using reserved table names like user.
	re = regexp.MustCompile(`(?msU)^(INSERT INTO) "?([a-zA-Z0-9_]*)"? (VALUES.*;)$`)
	sanitized = re.ReplaceAll(sanitized, []byte(`$1 "$2" $3`))

	return ioutil.WriteFile(dumpFile, sanitized, 0644)
}

// normalizeBackticksOutsideStringLiterals converts backticks to double quotes
// only when they are not inside single-quoted SQL string literals.
func normalizeBackticksOutsideStringLiterals(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	out := make([]byte, 0, len(data))
	inString := false

	for i := 0; i < len(data); i++ {
		b := data[i]

		if b == '\'' {
			out = append(out, b)
			if inString {
				// SQLite escapes single quotes inside strings as doubled quotes: ''
				if i+1 < len(data) && data[i+1] == '\'' {
					out = append(out, data[i+1])
					i++
					continue
				}
				inString = false
			} else {
				inString = true
			}
			continue
		}

		if b == '`' && !inString {
			out = append(out, '"')
			continue
		}

		out = append(out, b)
	}

	return out
}

// CustomSanitize allows you to expand upon the default Sanitize function
// by providing your own regex matcher and replacement to modify data from the dump file.
func CustomSanitize(dumpFile string, regex string, replacement []byte) error {
	re := regexp.MustCompile(regex)
	data, err := ioutil.ReadFile(dumpFile)
	if err != nil {
		return err
	}

	sanitized := re.ReplaceAll(data, replacement)

	return ioutil.WriteFile(dumpFile, sanitized, 0644)

}

// InjectColumnNames modifies INSERT statements in a dump file to include
// explicit column names sourced from the SQLite database. This ensures values
// are mapped to the correct Postgres columns even when column order differs.
func InjectColumnNames(dbFile string, dumpFile string) error {
	data, err := ioutil.ReadFile(dumpFile)
	if err != nil {
		return err
	}

	// Find all unique table names from INSERT statements (after Sanitize, format
	// is: INSERT INTO "tablename" VALUES)
	tableRe := regexp.MustCompile(`(?m)^INSERT INTO "(\w+)" VALUES`)
	tableSet := make(map[string]struct{})
	for _, m := range tableRe.FindAllSubmatch(data, -1) {
		tableSet[string(m[1])] = struct{}{}
	}

	if len(tableSet) == 0 {
		return nil
	}

	// Build column list string for each table
	tableColLists := make(map[string]string)
	for table := range tableSet {
		cols, err := getTableColumns(dbFile, table)
		if err != nil {
			return fmt.Errorf("getting columns for table %s: %w", table, err)
		}
		if len(cols) == 0 {
			continue
		}
		quoted := make([]string, len(cols))
		for i, c := range cols {
			quoted[i] = `"` + c + `"`
		}
		tableColLists[table] = "(" + strings.Join(quoted, ", ") + ")"
	}

	// Inject column names: INSERT INTO "t" VALUES -> INSERT INTO "t" (cols) VALUES
	content := tableRe.ReplaceAllFunc(data, func(match []byte) []byte {
		sub := tableRe.FindSubmatch(match)
		table := string(sub[1])
		colList, ok := tableColLists[table]
		if !ok {
			return match
		}
		return []byte(`INSERT INTO "` + table + `" ` + colList + ` VALUES`)
	})

	return ioutil.WriteFile(dumpFile, content, 0644)
}

// getTableColumns returns column names for a SQLite table in their defined order
// by querying PRAGMA table_info.
func getTableColumns(dbFile string, table string) ([]string, error) {
	cmd := exec.Command("sqlite3", dbFile, fmt.Sprintf(`PRAGMA table_info("%s");`, table))
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var cols []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// PRAGMA table_info output: cid|name|type|notnull|dflt_value|pk
		parts := strings.SplitN(line, "|", 3)
		if len(parts) >= 2 {
			cols = append(cols, parts[1])
		}
	}
	return cols, nil
}

// RemoveCreateStatements takes all the CREATE statements out of a dump
// so that no new tables are created.
func RemoveCreateStatements(dumpFile string) error {
	re := regexp.MustCompile(`(?msU)[\r\n]+^CREATE.*;$`)
	data, err := ioutil.ReadFile(dumpFile)
	if err != nil {
		return err
	}
	sanitized := re.ReplaceAll(data, nil)
	return ioutil.WriteFile(dumpFile, sanitized, 0644)
}

// HexDecode takes a file path containing a SQLite dump and
// decodes any hex-encoded data it finds.
func HexDecode(dumpFile string) error {
	re := regexp.MustCompile(`(?m)X\'([a-fA-F0-9]+)\'`)
	data, err := ioutil.ReadFile(dumpFile)
	if err != nil {
		return err
	}

	// Define a function to wrap encoded hex data in a call to decode hexstring.
	decodeHex := func(hexEncoded []byte) []byte {
		return []byte(fmt.Sprintf("convert_from('%s%s', 'utf-8')", `\x`, re.FindSubmatch(hexEncoded)[1]))
	}

	// Replace regex matches from the dumpFile using the `decodeHex` function defined above.
	sanitized := re.ReplaceAllFunc(data, decodeHex)
	return ioutil.WriteFile(dumpFile, sanitized, 0644)
}
