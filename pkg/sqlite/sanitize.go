package sqlite

import (
	"encoding/hex"
	"io/ioutil"
	"regexp"

	"github.com/sirupsen/logrus"
)

// Sanitize cleans up a SQLite dump file to prep it for import into Postgres.
func Sanitize(dumpFile string) error {
	// Change ` to "
	re := regexp.MustCompile("`")
	data, err := ioutil.ReadFile(dumpFile)
	if err != nil {
		return err
	}
	sanitized := re.ReplaceAll(data, []byte("\""))

	// Remove SQLite-specific PRAGMA statements
	// and statements that start with BEGIN
	// and statements pertaining to the sqlite_sequence table.
	re = regexp.MustCompile(`(?m)[\r\n]?^(PRAGMA.*;|BEGIN.*;|.*sqlite_sequence.*;)$`)
	sanitized = re.ReplaceAll(sanitized, nil)

	// Ensure there are quotes around table names to avoid using reserved table names like user.
	re = regexp.MustCompile(`(?msU)^(INSERT INTO) "?([a-zA-Z0-9_]*)"? (VALUES.*;)$`)
	sanitized = re.ReplaceAll(sanitized, []byte(`$1 "$2" $3`))

	return ioutil.WriteFile(dumpFile, sanitized, 0644)
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
	re2 := regexp.MustCompile(`'`)
	data, err := ioutil.ReadFile(dumpFile)
	if err != nil {
		return err
	}

	// Define a function to actually decode hexstring.
	decodeHex := func(hexEncoded []byte) []byte {
		// Find the regex submatch in the argument passed to the function
		// then decode the submatch.
		decoded, err := hex.DecodeString(string(re.FindSubmatch(hexEncoded)[1]))
		if err != nil {
			logrus.Fatalf("Failed to decode hex-string in: %s", hexEncoded)
		}
		decoded = re2.ReplaceAll(decoded, []byte(`''`))

		// Surround decoded string with single quotes again.
		return []byte(`'` + string(decoded) + `'`)
	}

	// Replace regex matches from the dumpFile using the `decodeHex` function defined above.
	sanitized := re.ReplaceAllFunc(data, decodeHex)
	return ioutil.WriteFile(dumpFile, sanitized, 0644)
}
