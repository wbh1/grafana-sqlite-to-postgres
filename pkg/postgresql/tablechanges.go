package postgresql

type Column struct {
	// Name of the column for the table
	Name string

	// If the column gets a default value set, specify it here
	Default string
}

// TableChange documents a table that needs to be changed
// and specificly which Columns need to be changed.
type TableChange struct {
	Table   string
	Columns []Column
}

var TableChanges = []TableChange{
	{
		Table: "alert",
		Columns: []Column{
			{
				Name: "silenced",
			},
		},
	},
	{
		Table: "alert_configuration",
		Columns: []Column{
			{
				Name:    "\"default\"",
				Default: "false",
			},
		},
	},
	{
		Table: "alert_configuration_history",
		Columns: []Column{
			{
				Name:    "\"default\"",
				Default: "false",
			},
		},
	},
	{
		Table: "alert_rule",
		Columns: []Column{
			{
				Name:    "is_paused",
				Default: "false",
			},
		},
	},
	{
		Table: "alert_rule_version",
		Columns: []Column{
			{
				Name:    "is_paused",
				Default: "false",
			},
		},
	},
	{
		Table: "alert_notification",
		Columns: []Column{
			{
				Name:    "is_default",
				Default: "false",
			},
			{
				Name:    "send_reminder",
				Default: "false",
			},
			{
				Name:    "disable_resolve_message",
				Default: "false",
			},
		},
	},
	{
		Table: "dashboard",
		Columns: []Column{
			{
				Name:    "is_folder",
				Default: "false",
			},
			{
				Name:    "has_acl",
				Default: "false",
			},
			{
				Name:    "is_public",
				Default: "false",
			},
		},
	},
	{
		Table: "dashboard_snapshot",
		Columns: []Column{
			{
				Name: "external",
			},
		},
	},
	{
		Table: "data_source",
		Columns: []Column{
			{
				Name: "basic_auth",
			},
			{
				Name: "is_default",
			},
			{
				Name: "read_only",
			},
			{
				Name:    "with_credentials",
				Default: "false",
			},
		},
	},
	{
		Table: "migration_log",
		Columns: []Column{
			{
				Name: "success",
			},
		},
	},
	{
		Table: "plugin_setting",
		Columns: []Column{
			{
				Name: "enabled",
			},
			{
				Name: "pinned",
			},
		},
	},
	{
		Table: "team_member",
		Columns: []Column{
			{
				Name: "external",
			},
		},
	},
	{
		Table: "temp_user",
		Columns: []Column{
			{
				Name: "email_sent",
			},
		},
	},
	{
		Table: "\"user\"",
		Columns: []Column{
			{
				Name: "is_admin",
			},
			{
				Name: "email_verified",
			},
			{
				Name:    "is_disabled",
				Default: "false",
			},
			{
				Name:    "is_service_account",
				Default: "false",
			},
		},
	},
	{
		Table: "user_auth_token",
		Columns: []Column{
			{
				Name: "auth_token_seen",
			},
		},
	},
	{
		Table: "role",
		Columns: []Column{
			{
				Name:    "hidden",
				Default: "false",
			},
		},
	},
	{
		Table: "data_keys",
		Columns: []Column{
			{
				Name: "active",
			},
		},
	},
	{
		Table: "api_key",
		Columns: []Column{
			{
				Name:    "is_revoked",
				Default: "false",
			},
		},
	},
}
