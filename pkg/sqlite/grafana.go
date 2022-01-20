package sqlite

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

type dashboard struct {
	slug     string
	folderId int
}

func getFolders(dbFile string) (map[int]string, error) {
	folders := make(map[int]string)
	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query("SELECT id,slug FROM dashboard WHERE is_folder=1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folderId int
	var folderSlug string
	for rows.Next() {
		err = rows.Scan(&folderId, &folderSlug)
		if err != nil {
			return nil, err
		}
		folders[folderId] = folderSlug
	}

	return folders, nil
}

func GetFoldersForDashboards(dbFile string) (map[string]string, error) {
	folders, err := getFolders(dbFile)
	if err != nil {
		return nil, err
	}

	dashboards := make(map[string]string)
	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query("SELECT slug,folder_id FROM dashboard WHERE is_folder=0")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dashboard dashboard
	for rows.Next() {
		err = rows.Scan(&dashboard.slug, &dashboard.folderId)
		if err != nil {
			return nil, err
		}
		if dashboard.folderId != 0 {
			folderSlug := folders[dashboard.folderId]
			dashboards[dashboard.slug] = folderSlug
		}
	}

	return dashboards, nil
}
