package main

import (
	"os"
	"time"

	"github.com/wbh1/grafana-sqlite-to-postgres/pkg/postgresql"
	"github.com/wbh1/grafana-sqlite-to-postgres/pkg/sqlite"

	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	log        = logrus.New()
	app        = kingpin.New("Grafana SQLite to Postgres Migrator", "A command-line application to migrate Grafana data from SQLite to Postgres.")
	dump       = app.Flag("dump", "Directory path where the sqlite dump should be stored.").Default("/tmp").ExistingDir()
	sqlitefile = app.Arg("sqlite-file", "Path to SQLite file being imported.").Required().File()
	connstring = app.Arg("postgres-connection-string", "URL-format database connection string to use in the URL format (postgres://USERNAME:PASSWORD@HOST/DATABASE).").Required().String()
	debug      = app.Flag("debug", "Enable debug level logging").Bool()
	progress   = app.Flag("progress-interval-seconds", "How often to log import progress in seconds. Set to 0 to disable periodic updates.").Default("60").Int()
	batchSize  = app.Flag("insert-batch-size", "How many same-shape INSERT statements to combine into one multi-row INSERT.").Default("200").Int()
)

func main() {

	kingpin.MustParse(app.Parse(os.Args[1:]))
	log.SetFormatter(&logrus.TextFormatter{
		DisableLevelTruncation: true,
		FullTimestamp:          true,
	})

	if *debug == true {
		log.SetLevel(logrus.DebugLevel)
	}

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

	// Inject explicit column names into INSERT statements so that values are
	// mapped correctly when Postgres column order differs from SQLite.
	if err := sqlite.InjectColumnNames(f.Name(), dumpPath); err != nil {
		log.Fatalf("❌ %v - failed to inject column names into dump file.", err)
	}
	log.Infoln("✅ Column names injected into INSERT statements")

	// Don't bother adding anything to the migration_log table.
	if err := sqlite.CustomSanitize(dumpPath, `(?msU)[\r\n]+^.*"migration_log.*;$`, nil); err != nil {
		log.Fatalf("❌ %v - failed to perform additional sanitizing of the dump file.", err)
	}
	log.Infoln("✅ migration_log statements removed")
	// Fix char conversion (char -> chr)
	if err := sqlite.CustomSanitize(dumpPath, `char\(10\)\)`, []byte("chr(10))")); err != nil {
		log.Fatalf("❌ %v - failed to perform char (LF) keyword sanitizing of the dump file.", err)
	}
	if err := sqlite.CustomSanitize(dumpPath, `char\(13\)\)`, []byte("chr(13))")); err != nil {
		log.Fatalf("❌ %v - failed to perform char (CR) keyword sanitizing of the dump file.", err)
	}
	log.Infoln("✅ char keyword transformed")

	// Do HexDecoding
	if err := sqlite.HexDecode(dumpPath); err != nil {
		log.Fatalf("❌ %v - failed to wrap hex-encoded values in the dump file.", err)
	}
	log.Infoln("✅ hex-encoded data values wrapped for insertion")

	// Connect to Postgres
	db, err := postgresql.New(*connstring, log)
	if err != nil {
		log.Fatalf("❌ %v - failed to connect to Postgres database.", err)
	}

	// Clear all rows from tables before importing
	tablesCleared, rowsDeleted, err := db.ClearTableRows()
	if err != nil {
		log.Fatalf("❌ %v - failed to clear table rows.", err)
	}
	log.Infof("✅ Cleared %d rows from %d tables (skipped migration_log)", rowsDeleted, tablesCleared)

	// Import the now-sanitized dump file into Postgres
	log.Infoln("🚚 Importing dump file to Postgres (this may take a while)")
	if err := db.ImportDump(dumpPath, time.Duration(*progress)*time.Second, *batchSize); err != nil {
		log.Fatalf("❌ %v - failed to import dump file to Postgres.", err)
	}
	log.Infoln("✅ Imported dump file to Postgres")
	log.Infoln("🎉 All done!")

}
