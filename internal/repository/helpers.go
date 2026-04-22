package repository

import (
	"database/sql"
	"time"
)

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func intBool(value int) bool {
	return value != 0
}

func now() time.Time {
	return time.Now().In(time.Local).Truncate(time.Second)
}

func scanTime(value any) time.Time {
	switch v := value.(type) {
	case time.Time:
		return v.In(time.Local)
	case string:
		t, _ := time.ParseInLocation(time.RFC3339, v, time.Local)
		return t
	case []byte:
		t, _ := time.ParseInLocation(time.RFC3339, string(v), time.Local)
		return t
	default:
		return time.Time{}
	}
}

func nullableString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}
