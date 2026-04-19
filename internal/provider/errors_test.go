package provider

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
)

func TestToGRPCCode(t *testing.T) {
	cases := []struct {
		in   error
		want codes.Code
	}{
		{ErrUnavailable, codes.Unavailable},
		{ErrRateLimited, codes.ResourceExhausted},
		{ErrInvalidArgument, codes.InvalidArgument},
		{ErrInternal, codes.Internal},
		{fmt.Errorf("wrap: %w", ErrUnavailable), codes.Unavailable},
		{nil, codes.OK},
		{errors.New("random"), codes.Internal},
	}
	for _, tc := range cases {
		got := ToGRPCCode(tc.in)
		if got != tc.want {
			t.Errorf("ToGRPCCode(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
