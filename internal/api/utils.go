package api

import "google.golang.org/protobuf/types/known/emptypb"

func empty() (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
