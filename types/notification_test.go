package types_test

import (
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/nakamauwu/nakama/types"
)

func TestNotificationExists_IsEmpty(t *testing.T) {
	assert.True(t, (types.NotificationExists{}).IsEmpty())
	assert.True(t, (types.NotificationExists{UserID: nil}).IsEmpty())
	assert.False(t, (types.NotificationExists{UserID: new("test")}).IsEmpty())
}
