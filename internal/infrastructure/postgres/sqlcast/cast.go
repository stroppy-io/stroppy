package sqlcast

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// PgTextToStr converts a pgtype.Text value to a string.
//
// It takes a pgtype.Text parameter named "text" and returns a string.
// If the "text" parameter is not valid, it returns an empty string.
// Otherwise, it returns the string value of the "text" parameter.
func PgTextToStr(text pgtype.Text) string {
	if !text.Valid {
		return ""
	}

	return text.String
}

// PgTextToStrPtr converts a pgtype.Text value to a pointer to a string.
//
// It takes a pgtype.Text parameter named "text" and returns a pointer to a string.
// If the "text" parameter is not valid, it returns nil.
// Otherwise, it returns a pointer to the string value of the "text" parameter.
func PgTextToStrPtr(text pgtype.Text) *string {
	if !text.Valid {
		return nil
	}

	return &text.String
}

// StrToPgText converts a string to a pgtype.Text value.
//
// It takes a string parameter named "str" and returns a pgtype.Text value.
// The pgtype.Text value is created with the string value of the input parameter and a valid flag set to true.
func StrToPgText(str string) pgtype.Text {
	return pgtype.Text{
		String: str,
		Valid:  true,
	}
}

// StrPrtToPgText converts a pointer to a string to a pgtype.Text.
//
// It takes a pointer to a string parameter named "str" and returns a pgtype.Text.
// If the "str" parameter is nil, it returns a pgtype.Text with the Valid flag set to false.
// Otherwise, it returns a pgtype.Text with the String field set to
// the value of the "str" parameter and the Valid flag set to true.
func StrPrtToPgText(str *string) pgtype.Text {
	if str == nil {
		return pgtype.Text{Valid: false}
	}

	return pgtype.Text{
		String: *str,
		Valid:  true,
	}
}

// PgUUIDToUUID converts a pgtype.UUID to a uuid.UUID.
//
// It takes a pgtype.UUID parameter named "uid" and returns a uuid.UUID.
func PgUUIDToUUID(uid pgtype.UUID) uuid.UUID {
	if !uid.Valid {
		return uuid.Nil
	}

	return uid.Bytes
}

// PgUUIDToUUIDPtr converts a pgtype.UUID to a pointer to a uuid.UUID.
//
// It takes a pgtype.UUID parameter named "uid" and returns a pointer to a uuid.UUID.
// If the "uid" parameter is not valid, it returns nil.
// Otherwise, it creates a new uuid.UUID using the Bytes field of the "uid" parameter and returns a pointer to it.
func PgUUIDToUUIDPtr(uid pgtype.UUID) *uuid.UUID {
	if !uid.Valid {
		return nil
	}

	u := uuid.UUID(uid.Bytes)

	return &u
}

// UUIDToPgUUID converts a uuid.UUID to a pgtype.UUID.
//
// It takes a uuid.UUID parameter named "uid" and returns a pgtype.UUID.
func UUIDToPgUUID(uid uuid.UUID) pgtype.UUID {
	return pgtype.UUID{
		Bytes: uid,
		Valid: true,
	}
}

// UUIDPtrToPgUUID converts a pointer to a uuid.UUID to a pgtype.UUID.
//
// It takes a pointer to a uuid.UUID parameter named "uid" and returns a pgtype.UUID.
func UUIDPtrToPgUUID(uid *uuid.UUID) pgtype.UUID {
	if uid == nil {
		return pgtype.UUID{Valid: false}
	}

	return pgtype.UUID{
		Bytes: *uid,
		Valid: true,
	}
}

// PgTimestamptzToTime converts a pgtype.Timestamptz to a time.Time.
//
// It takes a pgtype.Timestamptz parameter named "ts" and returns a time.Time.
func PgTimestamptzToTime(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}

	return ts.Time
}

// PgTimestamptzToTimePtr converts a pgtype.Timestamptz to a pointer to a time.Time.
//
// It takes a pgtype.Timestamptz parameter named "ts" and returns a pointer to a time.Time.
// If the "ts" parameter is not valid, it returns nil.
// Otherwise, it returns a pointer to the Time field of the "ts" parameter.
func PgTimestamptzToTimePtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}

	return &ts.Time
}

// TimeToPgTimestamptz converts a time.Time value to a pgtype.Timestamptz value.
//
// It takes a time.Time parameter named "t" and returns a pgtype.Timestamptz value.
// The returned pgtype.Timestamptz value has the Time field set to the input time value
// and the Valid field set to true.
//
// Parameters:
// - t: The time.Time value to convert.
//
// Returns:
// - pgtype.Timestamptz: The converted pgtype.Timestamptz value.
func TimeToPgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{
		Time:  t,
		Valid: true,
	}
}

// TimePtrToPgTimestamptz converts a pointer to a time.Time to a pgtype.Timestamptz.
//
// It takes a pointer to a time.Time parameter named "t" and returns a pgtype.Timestamptz.
// If the pointer is nil, it returns a pgtype.Timestamptz with the Valid field set to false.
// Otherwise, it returns a pgtype.Timestamptz with the Time field set to the value of the pointer
// and the Valid field set to true.
//
// Parameters:
// - t: A pointer to a time.Time.
//
// Returns:
// - pgtype.Timestamptz: The converted pgtype.Timestamptz.
func TimePtrToPgTimestamptz(castTime *time.Time) pgtype.Timestamptz {
	if castTime == nil {
		return pgtype.Timestamptz{Valid: false}
	}

	return pgtype.Timestamptz{
		Time:  *castTime,
		Valid: true,
	}
}
