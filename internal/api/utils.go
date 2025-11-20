package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgres/sqlexec"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

func empty() (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func emptyErr(err error) (*emptypb.Empty, error) {
	return nil, err
}

func dbGetter(executor sqlexec.Executor) orm.DbGetter {
	return func(ctx context.Context, operation orm.SqlOpType) orm.DB {
		return executor
	}
}

func timestamptzToTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func replaceMultipleStrings(str string, replacements map[string]string) string {
	for old, news := range replacements {
		str = strings.ReplaceAll(str, old, news)
	}
	return str
}

func NewUsersRepository(executor sqlexec.Executor) orm.UserRepository {
	return orm.NewUserRepository(
		dbGetter(executor),
		ids.UlidFromString,
		ids.UlidToStr,
	)
}

func NewRefreshTokensRepository(executor sqlexec.Executor) orm.RefreshTokensRepository {
	return orm.NewRefreshTokensRepository(
		dbGetter(executor),
		ids.UlidFromString,
		ids.UlidToStr,
	)
}

func NewRunRecordRepository(executor sqlexec.Executor) orm.RunRecordRepository {
	return orm.NewRunRecordRepository(
		dbGetter(executor),
		ids.UlidFromString,
		ids.UlidToStr,
		ids.UlidFromString,
		ids.UlidToStr,
		ids.UlidFromStringPtr,
		ids.UlidToStrPtr,
	)
}

func NewRunRecordStepRepository(executor sqlexec.Executor) orm.RunRecordStepRepository {
	return orm.NewRunRecordStepRepository(
		dbGetter(executor),
		ids.UlidFromString,
		ids.UlidToStr,
		ids.UlidFromString,
		ids.UlidToStr,
	)
}

func serializeTag(tag *panel.Tag) string {
	return fmt.Sprintf("%s:%s", tag.GetKey(), tag.GetValue())
}
func deserializeTag(tagStr string) *panel.Tag {
	parts := strings.Split(tagStr, ":")
	return &panel.Tag{
		Key:   parts[0],
		Value: parts[1],
	}
}

func serializeTags(tags []*panel.Tag) []string {
	return lo.Map(tags, func(t *panel.Tag, _ int) string {
		return serializeTag(t)
	})
}

func deserializeTags(tagStrs []string) []*panel.Tag {
	return lo.Map(tagStrs, func(tagStr string, _ int) *panel.Tag {
		return deserializeTag(tagStr)
	})
}

func NewTemplateRepository(executor sqlexec.Executor) orm.TemplateRepository {
	return orm.NewTemplateRepository(
		dbGetter(executor),
		ids.UlidFromString,
		ids.UlidToStr,
		ids.UlidFromString,
		ids.UlidToStr,
		deserializeTags,
		serializeTags,
	)
}

func NewKvTableRepository(executor sqlexec.Executor) orm.KvTableRepository {
	return orm.NewKvTableRepository(
		dbGetter(executor),
	)
}
