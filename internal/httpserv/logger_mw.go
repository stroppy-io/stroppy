package httpserv

import (
	"context"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"time"

	"connectrpc.com/connect"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func loggerMiddleware(logger *zap.Logger) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (res connect.AnyResponse, err error) {
			start := time.Now()
			resp, err := next(ctx, req)
			level := zapcore.InfoLevel
			if err != nil {
				level = zapcore.ErrorLevel
			}
			anyMsg, _ := protojson.Marshal(req.Any().(proto.Message))
			logger.Log(
				level,
				"request served",
				zap.String("method", req.HTTPMethod()),
				zap.String("peer_addr", req.Peer().Addr),
				zap.String("query", req.Peer().Query.Encode()),
				zap.String("protocol", req.Peer().Protocol),
				zap.String("procedure", req.Spec().Procedure),
				zap.String("idempotency_level", req.Spec().IdempotencyLevel.String()),
				zap.ByteString("anyMsg", anyMsg),
				zap.String("stream_type", req.Spec().StreamType.String()),
				zap.Duration("duration", time.Since(start)),
			)
			return resp, err
		}
	}
}
