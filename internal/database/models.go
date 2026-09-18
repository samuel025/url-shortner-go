package database

import "database/sql"

type Models struct {
	Users UserModel
	URLs  URLModel
}

func NewModels(db *sql.DB) Models {
	return Models{
		Users: UserModel{DB: db},
		URLs:  URLModel{DB: db},
	}
}
