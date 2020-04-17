package postgresql

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"strings"

	// Postgres driver
	_ "github.com/lib/pq"
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
	// TODO: Update this to use tablechanges.go
	changes := `ALTER TABLE alert ALTER COLUMN silenced TYPE integer USING silenced::integer;
	ALTER TABLE alert_notification ALTER COLUMN is_default DROP DEFAULT;
	ALTER TABLE alert_notification ALTER COLUMN is_default TYPE integer USING is_default::integer;
	ALTER TABLE alert_notification ALTER COLUMN send_reminder DROP DEFAULT;
	ALTER TABLE alert_notification ALTER COLUMN send_reminder TYPE integer USING send_reminder::integer;
	ALTER TABLE alert_notification ALTER COLUMN disable_resolve_message DROP DEFAULT;
	ALTER TABLE alert_notification ALTER COLUMN disable_resolve_message TYPE integer USING disable_resolve_message::integer;
	ALTER TABLE dashboard ALTER COLUMN is_folder DROP DEFAULT;
	ALTER TABLE dashboard ALTER COLUMN is_folder TYPE integer USING is_folder::integer;
	ALTER TABLE dashboard ALTER COLUMN has_acl DROP DEFAULT;
	ALTER TABLE dashboard ALTER COLUMN has_acl TYPE integer USING has_acl::integer;
	ALTER TABLE dashboard_snapshot ALTER COLUMN external TYPE integer USING external::integer;
	ALTER TABLE data_source ALTER COLUMN basic_auth TYPE integer USING basic_auth::integer;
	ALTER TABLE data_source ALTER COLUMN is_default TYPE integer USING is_default::integer;
	ALTER TABLE data_source ALTER COLUMN read_only TYPE integer USING read_only::integer;
	ALTER TABLE data_source ALTER COLUMN with_credentials DROP DEFAULT;
	ALTER TABLE data_source ALTER COLUMN with_credentials TYPE integer USING with_credentials::integer;
	ALTER TABLE migration_log ALTER COLUMN success TYPE integer USING success::integer;
	ALTER TABLE plugin_setting ALTER COLUMN enabled TYPE integer USING enabled::integer;
	ALTER TABLE plugin_setting ALTER COLUMN pinned TYPE integer USING pinned::integer;
	ALTER TABLE team_member ALTER COLUMN external TYPE integer USING external::integer;
	ALTER TABLE temp_user ALTER COLUMN email_sent TYPE integer USING email_sent::integer;
	ALTER TABLE "user" ALTER COLUMN is_admin TYPE integer USING is_admin::integer;
	ALTER TABLE "user" ALTER COLUMN email_verified TYPE integer USING email_verified::integer;
	ALTER TABLE "user" ALTER COLUMN is_disabled DROP DEFAULT;
	ALTER TABLE "user" ALTER COLUMN is_disabled TYPE integer USING is_disabled::integer;
	ALTER TABLE user_auth_token ALTER COLUMN auth_token_seen TYPE integer USING auth_token_seen::integer;
	DELETE FROM org WHERE id=1`

	for _, stmt := range strings.Split(changes, "\n") {
		if _, err := db.conn.Exec(stmt); err != nil {
			return fmt.Errorf("%v %v", err.Error(), stmt)
		}
	}

	return nil
}

// Change columns back to boolean type by decoding their current values
func (db *DB) decodeBooleanColumns() error {
	changes := `
		ALTER TABLE alert
			ALTER COLUMN silenced TYPE boolean
			USING CASE WHEN silenced = 0 THEN FALSE
				WHEN silenced = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE alert_notification
			ALTER COLUMN is_default TYPE boolean
			USING CASE WHEN is_default = 0 THEN FALSE
				WHEN is_default = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE alert_notification
			ALTER COLUMN send_reminder TYPE boolean
			USING CASE WHEN send_reminder = 0 THEN FALSE
				WHEN send_reminder = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE alert_notification
			ALTER COLUMN disable_resolve_message TYPE boolean
			USING CASE WHEN disable_resolve_message = 0 THEN FALSE
				WHEN disable_resolve_message = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE alert_notification ALTER COLUMN is_default SET DEFAULT false;
		ALTER TABLE dashboard
			ALTER COLUMN is_folder TYPE boolean
			USING CASE WHEN is_folder = 0 THEN FALSE
				WHEN is_folder = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE dashboard
			ALTER COLUMN has_acl TYPE boolean
			USING CASE WHEN has_acl = 0 THEN FALSE
				WHEN has_acl = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE dashboard_snapshot
			ALTER COLUMN external TYPE boolean
			USING CASE WHEN external = 0 THEN FALSE
				WHEN external = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE data_source
			ALTER COLUMN basic_auth TYPE boolean
			USING CASE WHEN basic_auth = 0 THEN FALSE
				WHEN basic_auth = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE data_source
			ALTER COLUMN is_default TYPE boolean
			USING CASE WHEN is_default = 0 THEN FALSE
				WHEN is_default = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE data_source
			ALTER COLUMN read_only TYPE boolean
			USING CASE WHEN read_only = 0 THEN FALSE
				WHEN read_only = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE data_source
			ALTER COLUMN with_credentials TYPE boolean
			USING CASE WHEN with_credentials = 0 THEN FALSE
				WHEN with_credentials = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE data_source ALTER COLUMN with_credentials SET DEFAULT false;
		ALTER TABLE migration_log
			ALTER COLUMN success TYPE boolean
			USING CASE WHEN success = 0 THEN FALSE
				WHEN success = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE plugin_setting
			ALTER COLUMN enabled TYPE boolean
			USING CASE WHEN enabled = 0 THEN FALSE
				WHEN enabled = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE plugin_setting
			ALTER COLUMN pinned TYPE boolean
			USING CASE WHEN pinned = 0 THEN FALSE
				WHEN pinned = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE team_member
			ALTER COLUMN external TYPE boolean
			USING CASE WHEN external = 0 THEN FALSE
				WHEN external = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE temp_user
			ALTER COLUMN email_sent TYPE boolean
			USING CASE WHEN email_sent = 0 THEN FALSE
				WHEN email_sent = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE "user"
			ALTER COLUMN is_admin TYPE boolean
			USING CASE WHEN is_admin = 0 THEN FALSE
				WHEN is_admin = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE "user"
			ALTER COLUMN email_verified TYPE boolean
			USING CASE WHEN email_verified = 0 THEN FALSE
				WHEN email_verified = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE "user"
			ALTER COLUMN is_disabled TYPE boolean
			USING CASE WHEN is_disabled = 0 THEN FALSE
				WHEN is_disabled = 1 THEN TRUE
				ELSE NULL
				END;
		ALTER TABLE "user" ALTER COLUMN is_disabled SET DEFAULT false;
		ALTER TABLE user_auth_token
			ALTER COLUMN auth_token_seen TYPE boolean
				USING CASE WHEN auth_token_seen = 0 THEN FALSE
				WHEN auth_token_seen = 1 THEN TRUE
				ELSE NULL
				END;
		`

	for _, stmt := range strings.Split(changes, ";") {
		if _, err := db.conn.Exec(stmt); err != nil {
			return fmt.Errorf("%v %v", err.Error(), stmt)
		}
	}

	return nil
}

// Make sure that sequences are fine on the tables
func (db *DB) fixSequences() error {
	var (
		tables = []string{
			"alert",
			"alert_notification",
			"annotation",
			"api_key",
			"dashboard_acl",
			"dashboard",
			"dashboard_provisioning",
			"dashboard_snapshot",
			"dashboard_tag",
			"dashboard_version",
			"data_source",
			"login_attempt",
			"migration_log",
			"org",
			"org_user",
			"playlist",
			"playlist_item",
			"plugin_setting",
			"preferences",
			"quota",
			"star",
			"tag",
			"team",
			"team_member",
			"temp_user",
			"test_data",
			"user_auth",
		}
	)

	// ref: https://stackoverflow.com/a/3698777/452467
	for _, table := range tables {
		stmt := fmt.Sprintf("SELECT setval(pg_get_serial_sequence('%s', 'id'), coalesce(max(id),0) + 1, false) FROM %s;", table, table)
		_, err := db.conn.Exec(stmt)
		if err != nil {
			return fmt.Errorf("%v %v", err.Error(), stmt)
		}
	}

	// Fix user sequence
	if _, err := db.conn.Exec("SELECT setval('user_id_seq', (SELECT MAX(id)+1 FROM \"user\"));"); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			// Try again with different column name
			if _, err := db.conn.Exec("SELECT setval('user_id_seq1', (SELECT MAX(id)+1 FROM \"user\"));"); err != nil {
				return fmt.Errorf("%v %v", err.Error(), "(failed to fix user sequence)")
			}
		} else {
			return fmt.Errorf("%v %v", err.Error(), "(failed to fix user sequence)")
		}
	}

	return nil

}
