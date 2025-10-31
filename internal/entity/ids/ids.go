package ids

import (
	"fmt"
	"github.com/oklog/ulid/v2"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"strings"
)

type OIDC = string

func NewUlid() *panel.Ulid {
	return &panel.Ulid{
		Id: ulid.Make().String(),
	}
}

func UlidToStr(ulid *panel.Ulid) string {
	if ulid == nil {
		return ""
	}
	return ulid.GetId()
}

func UlidFromString(str string) *panel.Ulid {
	return &panel.Ulid{
		Id: str,
	}
}

func UlidToStrPtr(ulid *panel.Ulid) *string {
	if ulid == nil {
		return nil
	}
	val := ulid.GetId()
	return &val
}

func UlidFromStringPtr(str *string) *panel.Ulid {
	if str == nil {
		return nil
	}
	return UlidFromString(*str)
}

func ExtractFromMediaRoomID(roomId string) (*panel.Ulid, *panel.Ulid, error) {
	parts := strings.Split(roomId, "-")
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("invalid room id: %s", roomId)
	}
	return UlidFromString(parts[0]), UlidFromString(parts[1]), nil
}
