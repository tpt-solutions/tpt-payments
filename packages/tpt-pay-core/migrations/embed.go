// Package migrations holds the SQL schema migrations. They are embedded in the server binary so that
// `tpt-pay-server migrate` can apply them from the production image, with no repository checked out.
package migrations

import "embed"

// FS holds every *.sql migration in this directory, in filename order.
//
//go:embed *.sql
var FS embed.FS
