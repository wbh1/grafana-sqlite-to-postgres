package postgresql

import (
	"bufio"
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	// Postgres driver
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
)

// DB allows for interface methods.
// It just holds a connection pointer.
type DB struct {
	conn *sql.DB
	log  *logrus.Logger
}

// New returns a Postgres database connection.
func New(connString string, logger *logrus.Logger) (db DB, err error) {
	db.log = logger
	db.conn, err = sql.Open("postgres", connString)
	if err != nil {
		return
	}
	_, err = db.conn.Exec("SELECT 1")
	return
}

// ClearTableRows deletes all rows from all tables in the public schema, except migration_log.
// This ensures a clean slate before inserting records from the dump.
// Returns the number of tables cleared and total rows affected.
func (db *DB) ClearTableRows() (int, int64, error) {
	// Query for all table names in the public schema
	query := `
		SELECT tablename 
		FROM pg_tables 
		WHERE schemaname = 'public'
		ORDER BY tablename
	`

	rows, err := db.conn.Query(query)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to query table names: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return 0, 0, fmt.Errorf("failed to scan table name: %v", err)
		}
		tables = append(tables, tableName)
	}

	if err = rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("error iterating table names: %v", err)
	}

	if len(tables) == 0 {
		return 0, 0, nil
	}

	db.log.Debugf("Found %d tables in public schema", len(tables))

	// Disable foreign key checks temporarily
	if _, err := db.conn.Exec("SET CONSTRAINTS ALL DEFERRED"); err != nil {
		db.log.Warnf("Could not defer constraints: %v", err)
	}

	tablesCleared := 0
	var totalRowsAffected int64

	for _, table := range tables {
		// Skip migration_log table
		if table == "migration_log" {
			db.log.Debugf("Skipping migration_log table")
			continue
		}

		deleteQuery := fmt.Sprintf("DELETE FROM \"%s\"", table)
		result, err := db.conn.Exec(deleteQuery)
		if err != nil {
			db.log.Warnf("Could not delete from table %s: %v", table, err)
			continue
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			db.log.Debugf("Deleted %d rows from %s", rowsAffected, table)
			tablesCleared++
			totalRowsAffected += rowsAffected
		}
	}

	// Re-enable foreign key checks
	if _, err := db.conn.Exec("SET CONSTRAINTS ALL IMMEDIATE"); err != nil {
		db.log.Warnf("Could not restore constraints: %v", err)
	}

	return tablesCleared, totalRowsAffected, nil
}

// ImportDump imports a SQL dump file.
func (db *DB) ImportDump(dumpFile string, progressInterval time.Duration, insertBatchSize int) error {
	promptToContinue := func() bool {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("You seem to have encountered some errors. Would you still like to continue? [Y/n]: ")
		text, _ := reader.ReadString('\n')
		switch response := strings.ToLower(text); response {
		case "n\n":
			return false
		default:
			return true
		}
	}

	// Auto-detect boolean columns in PostgreSQL that need temporary integer conversion
	// because SQLite stores booleans as 0/1 integers.
	boolCols, err := db.detectBooleanColumns()
	if err != nil {
		return fmt.Errorf("failed to detect boolean columns: %w", err)
	}
	db.log.Debugf("Detected %d boolean columns requiring integer conversion", len(boolCols))
	for _, col := range boolCols {
		if col.defaultValue != "" {
			db.log.Debugf("  boolean column: %s.%s (default: %s)", col.table, col.column, col.defaultValue)
		} else {
			db.log.Debugf("  boolean column: %s.%s (no default)", col.table, col.column)
		}
	}

	if errorEncountered := db.prepareTables(boolCols); errorEncountered == true {
		if promptToContinue() != true {
			return fmt.Errorf("%s", "Stopping migration at user's request.")
		}
	}

	file, err := ioutil.ReadFile(dumpFile)
	if err != nil {
		return err
	}

	sqlStmts := strings.Split(string(file), ";\n")
	totalStmts := 0
	for _, stmt := range sqlStmts {
		if strings.TrimSpace(stmt) != "" {
			totalStmts++
		}
	}

	start := time.Now()
	lastProgress := start
	processedStmts := 0
	progressEnabled := progressInterval > 0
	currentTable := ""
	pendingBatchTable := ""
	pendingBatchPrefix := ""
	var pendingBatchValues []string
	var pendingBatchStatements []string

	var tx *sql.Tx
	beginTx := func() error {
		var beginErr error
		tx, beginErr = db.conn.Begin()
		if beginErr != nil {
			return fmt.Errorf("failed to start import transaction: %w", beginErr)
		}
		return nil
	}
	commitTx := func() error {
		if tx == nil {
			return nil
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return fmt.Errorf("failed to commit import transaction: %w", commitErr)
		}
		tx = nil
		return nil
	}

	if err := beginTx(); err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	flushPendingBatch := func() error {
		if len(pendingBatchStatements) == 0 {
			return nil
		}

		if len(pendingBatchStatements) == 1 {
			err := db.executeImportStatement(tx, pendingBatchStatements[0], true)
			pendingBatchTable = ""
			pendingBatchPrefix = ""
			pendingBatchValues = nil
			pendingBatchStatements = nil
			return err
		}

		batchStmt := pendingBatchPrefix + strings.Join(pendingBatchValues, ",")
		if err := db.executeImportStatement(tx, batchStmt, false); err != nil {
			db.log.Debugf("Batch execution failed for %s (%d statements), falling back to single-row inserts: %v", pendingBatchTable, len(pendingBatchStatements), err)
			for _, singleStmt := range pendingBatchStatements {
				if singleErr := db.executeImportStatement(tx, singleStmt, true); singleErr != nil {
					pendingBatchTable = ""
					pendingBatchPrefix = ""
					pendingBatchValues = nil
					pendingBatchStatements = nil
					return singleErr
				}
			}
		}

		pendingBatchTable = ""
		pendingBatchPrefix = ""
		pendingBatchValues = nil
		pendingBatchStatements = nil
		return nil
	}

	for _, stmt := range sqlStmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if isTransactionControlStatement(stmt) {
			continue
		}

		table := targetInsertTable(stmt)
		if table != "" {
			if currentTable != "" && currentTable != table {
				if err := flushPendingBatch(); err != nil {
					return err
				}
				if err := commitTx(); err != nil {
					return err
				}
				if err := beginTx(); err != nil {
					return err
				}
			}
			currentTable = table
		}

		processedStmts++

		batchTable, batchPrefix, batchValues, batchable := splitInsertStatementForBatch(stmt)
		if batchable {
			if len(pendingBatchStatements) > 0 && (pendingBatchTable != batchTable || pendingBatchPrefix != batchPrefix) {
				if err := flushPendingBatch(); err != nil {
					return err
				}
			}

			pendingBatchTable = batchTable
			pendingBatchPrefix = batchPrefix
			pendingBatchValues = append(pendingBatchValues, batchValues)
			pendingBatchStatements = append(pendingBatchStatements, stmt)

			if len(pendingBatchStatements) >= insertBatchSize {
				if err := flushPendingBatch(); err != nil {
					return err
				}
			}
		} else {
			if err := flushPendingBatch(); err != nil {
				return err
			}
			if err := db.executeImportStatement(tx, stmt, true); err != nil {
				return err
			}
		}

		if progressEnabled && time.Since(lastProgress) >= progressInterval {
			elapsed := time.Since(start).Round(time.Second)
			progress := 0.0
			if totalStmts > 0 {
				progress = (float64(processedStmts) / float64(totalStmts)) * 100
			}
			db.log.Infof("⏳ Import progress: %d/%d statements (%.1f%%), elapsed %s", processedStmts, totalStmts, progress, elapsed)
			lastProgress = time.Now()
		}
	}

	if err := flushPendingBatch(); err != nil {
		return err
	}

	db.log.Debugf("Import execution complete: %d/%d statements in %s", processedStmts, totalStmts, time.Since(start).Round(time.Second))

	if err := commitTx(); err != nil {
		return err
	}

	// Fix boolean columns that we converted before.
	if errorEncountered := db.decodeBooleanColumns(boolCols); errorEncountered == true {
		if promptToContinue() != true {
			return fmt.Errorf("%s", "Stopping migration at user's request.")
		}
	}

	// Fix sequences for new items.
	if err := db.fixSequences(); err != nil {
		return err
	}

	return nil

}

func (db *DB) executeImportStatement(tx *sql.Tx, stmt string, ignoreDuplicate bool) error {
	if _, err := tx.Exec("SAVEPOINT import_stmt"); err != nil {
		return fmt.Errorf("failed to create savepoint: %w", err)
	}

	if _, err := tx.Exec(stmt); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			if _, rbErr := tx.Exec("ROLLBACK TO SAVEPOINT import_stmt"); rbErr != nil {
				return fmt.Errorf("failed to rollback savepoint after duplicate key: %w", rbErr)
			}
			if _, relErr := tx.Exec("RELEASE SAVEPOINT import_stmt"); relErr != nil {
				return fmt.Errorf("failed to release savepoint after duplicate key: %w", relErr)
			}
			if ignoreDuplicate {
				return nil
			}
			return fmt.Errorf("%v (statement starts with: %.200s)", err.Error(), stmt)
		}

		if strings.Contains(err.Error(), "is of type bytea but expression is of type text") {
			db.log.Debugf("Failed to import because of type issue (%v). Trying to fix...\n", err.Error())
			if _, rbErr := tx.Exec("ROLLBACK TO SAVEPOINT import_stmt"); rbErr != nil {
				return fmt.Errorf("failed to rollback savepoint after bytea type issue: %w", rbErr)
			}
			rewrittenStmt := rewriteByteaImportStatement(stmt)
			if _, err := tx.Exec(rewrittenStmt); err != nil {
				if _, rbErr := tx.Exec("ROLLBACK TO SAVEPOINT import_stmt"); rbErr != nil {
					return fmt.Errorf("failed to rollback savepoint after bytea retry failure: %w", rbErr)
				}
				if _, relErr := tx.Exec("RELEASE SAVEPOINT import_stmt"); relErr != nil {
					return fmt.Errorf("failed to release savepoint after bytea retry failure: %w", relErr)
				}
				return fmt.Errorf("%v (statement starts with: %.200s)", err.Error(), rewrittenStmt)
			}
			if _, relErr := tx.Exec("RELEASE SAVEPOINT import_stmt"); relErr != nil {
				return fmt.Errorf("failed to release savepoint after bytea retry: %w", relErr)
			}
			return nil
		}

		if _, rbErr := tx.Exec("ROLLBACK TO SAVEPOINT import_stmt"); rbErr != nil {
			return fmt.Errorf("failed to rollback savepoint after statement error: %w", rbErr)
		}
		if _, relErr := tx.Exec("RELEASE SAVEPOINT import_stmt"); relErr != nil {
			return fmt.Errorf("failed to release savepoint after statement error: %w", relErr)
		}
		return fmt.Errorf("%v (statement starts with: %.200s)", err.Error(), stmt)
	}

	if _, err := tx.Exec("RELEASE SAVEPOINT import_stmt"); err != nil {
		return fmt.Errorf("failed to release savepoint after success: %w", err)
	}

	return nil
}

func rewriteByteaImportStatement(stmt string) string {
	return strings.Replace(
		strings.Replace(stmt, `,convert_from('\x`, ",decode('", 1),
		"'utf-8'", "'hex'", 1)
}

// targetInsertTable extracts a normalized table identifier from INSERT statements.
// It returns an empty string for non-INSERT statements.
func targetInsertTable(stmt string) string {
	trimmed := strings.TrimSpace(stmt)
	if trimmed == "" {
		return ""
	}

	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "insert into ") {
		return ""
	}

	rest := strings.TrimSpace(trimmed[len("insert into "):])
	if strings.HasPrefix(strings.ToLower(rest), "only ") {
		rest = strings.TrimSpace(rest[len("only "):])
	}
	if rest == "" {
		return ""
	}

	tableEnd := len(rest)
	for i, r := range rest {
		if r == ' ' || r == '\t' || r == '\n' || r == '(' {
			tableEnd = i
			break
		}
	}

	table := strings.TrimSpace(rest[:tableEnd])
	table = strings.ReplaceAll(table, `"`, "")
	return strings.ToLower(table)
}

func splitInsertStatementForBatch(stmt string) (table string, prefix string, values string, ok bool) {
	table = targetInsertTable(stmt)
	if table == "" {
		return "", "", "", false
	}

	trimmed := strings.TrimSpace(stmt)
	lower := strings.ToLower(trimmed)
	valuesIndex := strings.Index(lower, " values")
	if valuesIndex == -1 {
		return "", "", "", false
	}

	prefix = trimmed[:valuesIndex+len(" values")]
	values = strings.TrimSpace(trimmed[valuesIndex+len(" values"):])
	if !strings.HasPrefix(values, "(") {
		return "", "", "", false
	}

	return table, prefix, values, true
}

func isTransactionControlStatement(stmt string) bool {
	trimmed := strings.TrimSpace(stmt)
	if trimmed == "" {
		return false
	}

	lower := strings.ToLower(strings.TrimSuffix(trimmed, ";"))
	fields := strings.Fields(lower)
	if len(fields) == 0 {
		return false
	}

	if len(fields) == 1 {
		switch fields[0] {
		case "begin", "commit", "rollback", "end":
			return true
		}
	}

	if len(fields) == 2 && (fields[0] == "begin" || fields[0] == "end") && fields[1] == "transaction" {
		return true
	}

	if len(fields) == 2 && fields[0] == "rollback" && fields[1] == "transaction" {
		return true
	}

	return false
}

// boolColumn holds a PostgreSQL boolean column's location and default value.
type boolColumn struct {
	table        string // double-quoted identifier, e.g. "alert"
	column       string // double-quoted identifier, e.g. "silenced"
	defaultValue string // "true", "false", or "" when there is no default
}

// detectBooleanColumns queries PostgreSQL information_schema for all boolean columns
// in the public schema. These columns require temporary integer conversion during
// SQLite data import because SQLite stores booleans as 0 and 1.
func (db *DB) detectBooleanColumns() ([]boolColumn, error) {
	rows, err := db.conn.Query(`
		SELECT table_name, column_name, column_default
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND data_type = 'boolean'
		ORDER BY table_name, column_name
	`)
	if err != nil {
		return nil, fmt.Errorf("querying boolean columns: %w", err)
	}
	defer rows.Close()

	var cols []boolColumn
	for rows.Next() {
		var tableName, columnName string
		var columnDefault sql.NullString
		if err := rows.Scan(&tableName, &columnName, &columnDefault); err != nil {
			return nil, fmt.Errorf("scanning boolean column: %w", err)
		}
		col := boolColumn{
			table:  `"` + tableName + `"`,
			column: `"` + columnName + `"`,
		}
		if columnDefault.Valid {
			col.defaultValue = normalizeBoolDefault(columnDefault.String)
		}
		cols = append(cols, col)
	}
	return cols, rows.Err()
}

// normalizeBoolDefault converts a PostgreSQL column_default expression for a boolean
// column (e.g. "false" or "'false'::boolean") into a plain "true" or "false" literal.
func normalizeBoolDefault(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if idx := strings.Index(s, "::"); idx != -1 {
		s = strings.TrimSpace(s[:idx])
	}
	return strings.Trim(s, "'")
}

// prepareTables temporarily converts boolean columns to integer so that the
// SQLite dump's 0/1 values can be inserted without type errors.
func (db *DB) prepareTables(cols []boolColumn) (errorEncountered bool) {
	for _, col := range cols {
		if col.defaultValue != "" {
			stmt := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT", col.table, col.column)
			db.log.Debugln("Executing: ", stmt)
			if _, err := db.conn.Exec(stmt); err != nil {
				if strings.Contains(err.Error(), "does not exist") {
					db.log.Debugf("%s %v %v", "Column/table doesn't exist. This is usually fine to ignore, but here's the info:", err.Error(), stmt)
				} else {
					db.log.Warnf("%v %v", err.Error(), stmt)
					errorEncountered = true
				}
			}
		}

		stmt := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE integer USING %s::integer", col.table, col.column, col.column)
		db.log.Debugln("Executing: ", stmt)
		if _, err := db.conn.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "does not exist") {
				db.log.Debugf("%s %v %v", "Column/table doesn't exist. This is usually fine to ignore, but here's the info:", err.Error(), stmt)
			} else {
				db.log.Warnf("%v %v", err.Error(), stmt)
				errorEncountered = true
			}
		}
	}
	return
}

// decodeBooleanColumns converts the temporarily-integer columns back to boolean
// and restores any defaults that were dropped in prepareTables.
func (db *DB) decodeBooleanColumns(cols []boolColumn) bool {
	var errorEncountered bool

	for _, col := range cols {
		stmt := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE boolean USING CASE WHEN %s = 0 THEN FALSE WHEN %s = 1 THEN TRUE ELSE NULL END", col.table, col.column, col.column, col.column)
		db.log.Debugln("Executing: ", stmt)
		if _, err := db.conn.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "does not exist") {
				db.log.Debugf("%s %v %v", "Column/table doesn't exist. This is usually fine to ignore, but here's the info:", err.Error(), stmt)
			} else {
				db.log.Warnf("%v %v", err.Error(), stmt)
				errorEncountered = true
			}
		}

		if col.defaultValue != "" {
			stmt = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s", col.table, col.column, col.defaultValue)
			db.log.Debugln("Executing: ", stmt)
			if _, err := db.conn.Exec(stmt); err != nil {
				if strings.Contains(err.Error(), "does not exist") {
					db.log.Debugf("%s %v %v", "Column/table doesn't exist. This is usually fine to ignore, but here's the info:", err.Error(), stmt)
				} else {
					db.log.Warnf("%v %v", err.Error(), stmt)
					errorEncountered = true
				}
			}
		}
	}

	return errorEncountered
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

	db.log.Debugln("Running query to generate statements to reset all sequences.")
	rows, err := db.conn.Query(stmt)
	if err != nil {
		return fmt.Errorf("%v %v", err.Error(), stmt)
	}
	defer rows.Close()

	db.log.Debugln("Running generated queries to reset all sequences.")
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
