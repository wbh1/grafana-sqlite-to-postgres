package sqlite

import (
	"io"
	"io/ioutil"
	"os/exec"
	"regexp"
)

// Exists checks if `sqlite3 --version` returns without errors
func Exists() error {
	// Check to make sure sqlite3 exists
	cmd := exec.Command("sqlite3", "--version")
	return cmd.Run()
}

// Dump performs a full dump of the SQLite database
func Dump(dbFile string, destination string) error {

	cmd := exec.Command("sqlite3", dbFile)

	// Interact with sqlite3 command line tool by sending data to STDIN
	stdin, _ := cmd.StdinPipe()
	go func() {
		defer stdin.Close()
		io.WriteString(stdin, ".output "+destination+"\n")
		io.WriteString(stdin, ".dump\n")
		io.WriteString(stdin, ".quit\n")
	}()

	_, err := cmd.CombinedOutput()

	return err
}

// Sanitize cleans up a SQLite dump file to prep it for import into Postgres
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
	// and statements for the migration_log
	re = regexp.MustCompile(`(?m)^(PRAGMA.*;|BEGIN.*;|INSERT INTO migration_log.*;|.*sqlite_sequence.*;)$`)
	sanitized = re.ReplaceAll(sanitized, nil)

	// Put quotes around table names to avoid using reserved table names like user
	re = regexp.MustCompile(`(?msU)^(INSERT INTO) (.*) (VALUES.*;)$`)
	sanitized = re.ReplaceAll(sanitized, []byte(`$1 "$2" $3`))

	return ioutil.WriteFile(dumpFile, sanitized, 0644)
}

// RemoveCreateStatements takes all the CREATE statements out of a dump
// so that no new tables are created.
func RemoveCreateStatements(dumpFile string) error {
	re := regexp.MustCompile(`(?msU)^CREATE.*;$`)
	data, err := ioutil.ReadFile(dumpFile)
	if err != nil {
		return err
	}
	sanitized := re.ReplaceAll(data, nil)
	return ioutil.WriteFile(dumpFile, sanitized, 0644)
}
