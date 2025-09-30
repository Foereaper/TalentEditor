// Copyright (c) 2025 DBCTool
//
// DBCTool is licensed under the MIT License.
// See the LICENSE file for details.

package main

import (
    "database/sql"
    "fmt"

    _ "github.com/go-sql-driver/mysql"
)

// DBConnections holds open database connections
type DBConnections struct {
    DBC *sql.DB
}

// openDB opens a database connection from DBConfig
func openDB(c DBConfig) (*sql.DB, error) {
    dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
        c.User, c.Password, c.Host, c.Port, c.Name)

    db, err := sql.Open("mysql", dsn)
    if err != nil {
        return nil, fmt.Errorf("open db: %w", err)
    }

    if err := db.Ping(); err != nil {
        return nil, fmt.Errorf("ping db: %w", err)
    }

    return db, nil
}
