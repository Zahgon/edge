package jsrt

import (
	"time"
)

func init() {
	// The Node fixtures were recorded in the UTC time zone; local time must be
	// the same for the date cases to match.
	time.Local = time.UTC
}
