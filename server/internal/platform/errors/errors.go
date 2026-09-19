// Package platformerrors contains transport-independent infrastructure errors.
package platformerrors

import "errors"

// ErrTemporarilyUnavailable is returned when a database operation cannot
// complete within its configured bounded availability window. Callers should
// expose the project's 503 contract instead of a driver-specific message.
var ErrTemporarilyUnavailable = errors.New("service temporarily unavailable")
