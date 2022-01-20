package main

import (
	"os"

	"github.com/percona/grafana-db-migrator/pkg/postgresql"
	"github.com/percona/grafana-db-migrator/pkg/sqlite"

	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	log                = logrus.New()
	app                = kingpin.New("Grafana SQLite to Postgres Migrator", "A command-line application to migrate Grafana data from SQLite to Postgres.")
	dump               = app.Flag("dump", "Directory path where the sqlite dump should be stored.").Default("/tmp").ExistingDir()
	sqlitefile         = app.Arg("sqlite-file", "Path to SQLite file being imported.").Required().File()
	connstring         = app.Arg("postgres-connection-string", "URL-format database connection string to use in the URL format (postgres://USERNAME:PASSWORD@HOST/DATABASE).").Required().String()
	debug              = app.Flag("debug", "Enable debug level logging").Bool()
	resetHomeDashboard = app.Flag("reset-home-dashboard", "Reset home dashboard for default organization").Bool()
	changeCharToText   = app.Flag("change-char-to-text", "Change CHAR filed to TEXT").Bool()
)

func main() {

	kingpin.MustParse(app.Parse(os.Args[1:]))
	log.SetFormatter(&logrus.TextFormatter{
		DisableLevelTruncation: true,
		FullTimestamp:          true,
	})

	if *debug {
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

	// Don't bother adding anything to the migration_log table.
	if err := sqlite.CustomSanitize(dumpPath, `(?msU)[\r\n]+^.*"migration_log.*;$`, nil); err != nil {
		log.Fatalf("❌ %v - failed to perform additional sanitizing of the dump file.", err)
	}
	log.Infoln("✅ migration_log statements removed")
	// Fix char conversion (char -> chr)
	if err := sqlite.CustomSanitize(dumpPath, `char\(10\)\)`, []byte("chr(10))")); err != nil {
		log.Fatalf("❌ %v - failed to perform char keyword sanitizing of the dump file.", err)
	}
	log.Infoln("✅ char keyword transformed")

	// Do HexDecoding
	if err := sqlite.HexDecode(dumpPath); err != nil {
		log.Fatalf("❌ %v - failed to perform hex decoding of the dump file.", err)
	}
	log.Infoln("✅ hex-encoded data decoded")

	// Connect to Postgres
	db, err := postgresql.New(*connstring, log)
	if err != nil {
		log.Fatalf("❌ %v - failed to connect to Postgres database.", err)
	}

	// Import the now-sanitized dump file into Postgres
	if err := db.ImportDump(dumpPath); err != nil {
		log.Fatalf("❌ %v - failed to import dump file to Postgres.", err)
	}
	log.Infoln("✅ Imported dump file to Postgres")

	// Get folder/dashboard relationshio for fixing after upgrade
	dashboardFolders, err := sqlite.GetFoldersForDashboards(f.Name())
	if err != nil {
		log.Fatalf("❌ %v - failed to get relationship between folders and dashboards.", err)
	}
	log.Infoln("✅ got folder/dashboard relationship from SQLite")

	if err := db.FixFolderID(dashboardFolders, log); err != nil {
		log.Fatalf("❌ %v - failed to fix folders ID.", err)
	}
	log.Infoln("✅ folders ID was fixed")

	if *resetHomeDashboard {
		if err := db.FixHomeDashboard(); err != nil {
			log.Fatalf("❌ %v - failed to change home dashboard.", err)
		}
		log.Infoln("✅ home dashboard was changed to default.")
	}

	if *changeCharToText {
		if err := db.ChangeCharToText(); err != nil {
			log.Fatalf("❌ %v - failed convert CHAR type to TEXT")
		}
		log.Infoln("✅ CHAR type was converted to TEXT.")
	}
	log.Infoln("🎉 All done!")

}
