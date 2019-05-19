package main

import (
	"os"

	"github/wbh1/grafana-sqlite-to-postgres/pkg/postgresql"
	"github/wbh1/grafana-sqlite-to-postgres/pkg/sqlite"

	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	log        = logrus.New()
	app        = kingpin.New("Grafana SQLite to Postgres Migrator", "A command-line application to migrate Grafana data from SQLite to Postgres.")
	dump       = app.Flag("dump", "Directory path where the sqlite dump should be stored.").Default("/tmp").ExistingDir()
	sqlitefile = app.Arg("sqlite-file", "Path to SQLite file being imported.").Required().File()
	connstring = app.Arg("postgres-connection-string", "URL-format database connection string to use in the URL format (postgres://USERNAME:PASSWORD@HOST/DATABASE).").Required().String()
)

func main() {

	kingpin.MustParse(app.Parse(os.Args[1:]))
	log.SetFormatter(&logrus.TextFormatter{
		DisableLevelTruncation: true,
		FullTimestamp:          true,
	})

	dumpPath := *dump + "/grafana.sql"

	// Must dereference
	f := *sqlitefile
	log.Infof("📁 SQLlite file: %v", f.Name())
	log.Infof("📁 Dump directory: %v", *dump)

	// Make sure SQLite exists on machine
	if err := sqlite.Exists(); err != nil {
		log.Fatalf("❌ %v - is the sqlite3 command line tool installed?", err)
	}
	log.Infof("✅ sqlite3 command exists")

	// Dump the SQLite database
	if err := sqlite.Dump(f.Name(), dumpPath); err != nil {
		log.Fatalf("❌ %v - failed to dump database.", err)
	}
	log.Infof("✅ sqlite3 database dumped to %v", dumpPath)

	// Remove CREATE statements
	if err := sqlite.RemoveCreateStatements(dumpPath); err != nil {
		log.Fatalf("❌ %v - failed to remove CREATE statements from dump file.", err)
	}
	log.Infoln("✅ CREATE statements removed from dump file")

	// Sanitize the SQLite dump
	if err := sqlite.Sanitize(dumpPath); err != nil {
		log.Fatalf("❌ %v - failed to sanitize dump file.", err)
	}
	log.Infoln("✅ sqlite3 dump sanitized")

	// Don't bother adding anything to the migration_log table.
	if err := sqlite.CustomSanitize(dumpPath, `(?m)[\r\n]+^INSERT INTO "migration_log".*;$`, nil); err != nil {
		log.Fatalf("❌ %v - failed to perform additional sanitizing of the dump file.", err)
	}
	log.Infoln("✅ migration_log statements removed")

	// Do HexDecoding
	if err := sqlite.HexDecode(dumpPath); err != nil {
		log.Fatalf("❌ %v - failed to perform hex decoding of the dump file.", err)
	}
	log.Infoln("✅ hex-encoded data decoded")

	// Connect to Postgres
	db, err := postgresql.New(*connstring)
	if err != nil {
		log.Fatalf("❌ %v - failed to connect to Postgres database.", err)
	}

	// Import the now-sanitized dump file into Postgres
	if err := db.ImportDump(dumpPath); err != nil {
		log.Fatalf("❌ %v - failed to import dump file to Postgres.", err)
	}
	log.Infoln("✅ Imported dump file to Postgres")
	log.Infoln("🎉 All done!")

}
