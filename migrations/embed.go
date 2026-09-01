// Package migrations встраивает SQL-файлы миграций golang-migrate в
// бинарь приложения через go:embed. Файлы именуются по конвенции
// golang-migrate: {version}_{name}.up.sql / {version}_{name}.down.sql.
package migrations

import "embed"

// FS — встроенная файловая система с SQL-файлами миграций.
// Используется как источник iofs в internal/infrastructure/db/migrate.go.
//
//go:embed *.sql
var FS embed.FS
