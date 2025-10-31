package timestamps

import (
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func NewTiming() *panel.Timing {
	return &panel.Timing{
		CreatedAt: timestamppb.Now(),
		UpdatedAt: timestamppb.Now(),
		DeletedAt: nil,
	}
}
