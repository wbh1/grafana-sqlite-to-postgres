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
	user       = app.Flag("user", "Username to use to connect to Postgres.").Short('u').String()
	password   = app.Flag("password", "Password to use to connect to Postgres.").Short('p').String()
	host       = app.Flag("host", "TCP address (host:port) to connect to Postgres on.").Short('H').TCP()
	db         = app.Flag("db", "Name of the database to connect to.").Short('d').String()
	dump       = app.Flag("dump", "File path where the sqlite dump should be stored.").Default("/tmp").ExistingDir()
	connstring = app.Flag("connstring", "Optional database connection string to use in the URL format (postgres://USERNAME:PASSWORD@HOST/DATABASE). This overrides all other connection parameters.").Short('c').String()
	sqlitefile = app.Arg("sqlite-file", "Path to SQLite file being imported.").File()
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

	db, err := postgresql.New(*connstring)
	if err != nil {
		log.Fatalf("❌ %v - failed to connect to Postgres database.", err)
	}

	if err := db.ImportDump(dumpPath); err != nil {
		log.Fatalf("❌ %v - failed to import dump file to Postgres.", err)
	}
	log.Infoln("✅ Imported dump file to Postgres")

}
