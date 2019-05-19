package sqlite

import (
	"io"
	"os/exec"
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
