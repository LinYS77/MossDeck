package bookmark

import "time"

// dbTimeFormat is the SQLite TEXT timestamp format produced by
// datetime('now'); identical to the auth package's constant.
const dbTimeFormat = "2006-01-02 15:04:05"

const rfc3339 = time.RFC3339

// parseDBTime parses a SQLite datetime('now')-style timestamp.
func parseDBTime(s string) (time.Time, error) {
	return time.Parse(dbTimeFormat, s)
}
