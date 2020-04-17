package postgresql

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"strings"

	// Postgres driver
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
)

// DB allows for interface methods.
// It just holds a connection pointer.
type DB struct {
	conn *sql.DB
}

// New returns a Postgres database connection.
func New(connString string) (db DB, err error) {
	db.conn, err = sql.Open("postgres", connString)
	if err != nil {
		return
	}
	_, err = db.conn.Exec("SELECT 1")
	return
}

// ImportDump imports a SQL dump file.
func (db *DB) ImportDump(dumpFile string) error {

	// Alter tables because of boolean issues
	// SQLite has booleans as 1's and 0's
	// Postgres is true/false
	// We'll convert it after importing the dump.
	if err := db.prepareTables(); err != nil {
		return err
	}

	file, err := ioutil.ReadFile(dumpFile)
	if err != nil {
		return err
	}

	sqlStmts := strings.Split(string(file), ";\n")

	for _, stmt := range sqlStmts {
		if _, err := db.conn.Exec(stmt); err != nil {
			// We can safely ignore "duplicate key value violates unique constraint" errors.
			if strings.Contains(err.Error(), "duplicate key") {
				continue
			} else {
				return fmt.Errorf("%v %v", err.Error(), stmt)
			}
		}
	}

	// Fix boolean columns that we converted before.
	if err := db.decodeBooleanColumns(); err != nil {
		return err
	}

	// Fix sequences for new items.
	if err := db.fixSequences(); err != nil {
		return err
	}

	return nil

}

// Change column types that expect boolean to integer so that we can get the data in.
// We'll decode their values into booleans later.
func (db *DB) prepareTables() error {
	for _, table := range TableChanges {
		// for each column associated with the table,
		// update the column type to be integer so that it's compatible with sqlite's 0/1 bool values
		for _, column := range table.Columns {
			stmt := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE integer USING %s::integer", table.Table, column.Name, column.Name)
			logrus.Debugln("Executing: ", stmt)
			if _, err := db.conn.Exec(stmt); err != nil {
				return fmt.Errorf("%v %v", err.Error(), stmt)
			}

			// If the column has a default value associated with it, drop it.
			if column.Default != "" {
				stmt = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT", table.Table, column.Name)
				logrus.Debugln("Executing: ", stmt)
				if _, err := db.conn.Exec(stmt); err != nil {
					return fmt.Errorf("%v %v", err.Error(), stmt)
				}
			}

		}
	}

	// Delete the org that gets auto-generated the first time Grafana runs.
	stmt := "DELETE FROM org WHERE id=1"
	logrus.Debugln("Executing: ", stmt)
	if _, err := db.conn.Exec(stmt); err != nil {
		return fmt.Errorf("%v %v", err.Error(), stmt)
	}

	return nil
}

// Change columns back to boolean type by decoding their current values
func (db *DB) decodeBooleanColumns() error {

	for _, table := range TableChanges {
		for _, column := range table.Columns {
			stmt := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE boolean USING CASE WHEN %s = 0 THEN FALSE WHEN %s = 1 THEN TRUE ELSE NULL END", table.Table, column.Name, column.Name, column.Name)
			logrus.Debugln("Executing: ", stmt)
			if _, err := db.conn.Exec(stmt); err != nil {
				return fmt.Errorf("%v %v", err.Error(), stmt)
			}

			// If the column has a default value associated with it, drop it.
			if column.Default != "" {
				stmt = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s", table.Table, column.Name, column.Default)
				logrus.Debugln("Executing: ", stmt)
				if _, err := db.conn.Exec(stmt); err != nil {
					return fmt.Errorf("%v %v", err.Error(), stmt)
				}
			}

		} // end column loop
	} // end table loop

	return nil
}

// Make sure that sequences are fine on the tables
func (db *DB) fixSequences() error {

	// Query from https://wiki.postgresql.org/wiki/Fixing_Sequences
	stmt := `SELECT 'SELECT SETVAL(' ||
	quote_literal(quote_ident(PGT.schemaname) || '.' || quote_ident(S.relname)) ||
	', COALESCE(MAX(' ||quote_ident(C.attname)|| '), 1) ) FROM ' ||
	quote_ident(PGT.schemaname)|| '.'||quote_ident(T.relname)|| ';' stmt
FROM pg_class AS S,
pg_depend AS D,
pg_class AS T,
pg_attribute AS C,
pg_tables AS PGT
WHERE S.relkind = 'S'
AND S.oid = D.objid
AND D.refobjid = T.oid
AND D.refobjid = C.attrelid
AND D.refobjsubid = C.attnum
AND T.relname = PGT.tablename
ORDER BY S.relname;`

	logrus.Debugln("Running query to generate statements to reset all sequences.")
	rows, err := db.conn.Query(stmt)
	if err != nil {
		return fmt.Errorf("%v %v", err.Error(), stmt)
	}
	defer rows.Close()

	logrus.Debugln("Running generated queries to reset all sequences.")
	for rows.Next() {
		var stmt string
		if err := rows.Scan(&stmt); err != nil {
			return fmt.Errorf("%v %v", "Failed to retrieve sequence reset statement", err)
		}

		// Execute the generate statement
		if _, err := db.conn.Exec(stmt); err != nil {
			return fmt.Errorf("%v %v", err.Error(), stmt)
		}
	}

	return nil

}
