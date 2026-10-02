package runtime

import (
	"database/sql"
)

// Tx is one store transaction. It is valid only inside the Update callback that
// received it: the store commits or rolls the transaction back when the callback
// returns.
type Tx struct {
	tx *sql.Tx
}

// rowScanner is the common surface of *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}
