package provider

import (
	"errors"

	"google.golang.org/grpc/codes"
)

var (
	ErrUnavailable     = errors.New("provider: unavailable")
	ErrRateLimited     = errors.New("provider: rate limited")
	ErrInvalidArgument = errors.New("provider: invalid argument")
	ErrInternal        = errors.New("provider: internal")
)

func ToGRPCCode(err error) codes.Code {
	switch {
	case err == nil:
		return codes.OK
	case errors.Is(err, ErrUnavailable):
		return codes.Unavailable
	case errors.Is(err, ErrRateLimited):
		return codes.ResourceExhausted
	case errors.Is(err, ErrInvalidArgument):
		return codes.InvalidArgument
	case errors.Is(err, ErrInternal):
		return codes.Internal
	default:
		return codes.Internal
	}
}
