package http

import (
	"fmt"
	"testing"

	"github.com/nakamauwu/nakama/service"
	"github.com/nicolasparada/go-errs"
)

func Test_shouldLoginWithProviderRetry(t *testing.T) {
	tt := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "service_const_user_not_found",
			err:  service.ErrUserNotFound,
			want: true,
		},
		{
			name: "inline_not_found_error",
			err:  errs.NotFoundError("user not found"),
			want: true,
		},
		{
			name: "wrapped_service_const_user_not_found",
			err:  fmt.Errorf("some context: %w", service.ErrUserNotFound),
			want: true,
		},
		{
			name: "wrapped_inline_not_found_error",
			err:  fmt.Errorf("some context: %w", errs.NotFoundError("user not found")),
			want: true,
		},
		{
			name: "other_not_found",
			err:  errs.NotFoundError("some other not found error"),
			want: false,
		},
		{
			name: "other_error",
			err:  fmt.Errorf("some other error"),
			want: false,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldRetryLoginWithProvider(tc.err)
			if got != tc.want {
				t.Errorf("shouldLoginWithProviderRetry() = %v, want %v", got, tc.want)
			}
		})
	}
}
