package manager

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"

	"github.com/pomerium/pomerium/internal/directory"
	"github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/pomerium/pkg/grpc/session"
	"github.com/pomerium/pomerium/pkg/grpc/user"
	"github.com/pomerium/pomerium/pkg/protoutil"
)

type mockProvider struct {
	user       func(ctx context.Context, userID, accessToken string) (*directory.User, error)
	userGroups func(ctx context.Context) ([]*directory.Group, []*directory.User, error)
}

func (mock mockProvider) User(ctx context.Context, userID, accessToken string) (*directory.User, error) {
	return mock.user(ctx, userID, accessToken)
}

func (mock mockProvider) UserGroups(ctx context.Context) ([]*directory.Group, []*directory.User, error) {
	return mock.userGroups(ctx)
}

func TestManager_onUpdateRecords(t *testing.T) {
	ctx, clearTimeout := context.WithTimeout(context.Background(), time.Second*10)
	defer clearTimeout()

	now := time.Now()

	mgr := New(
		WithDirectoryProvider(mockProvider{}),
		WithGroupRefreshInterval(time.Hour),
		WithNow(func() time.Time {
			return now
		}),
	)
	mgr.directoryBackoff.RandomizationFactor = 0 // disable randomization for deterministic testing

	mgr.onUpdateRecords(ctx, updateRecordsMessage{
		records: []*databroker.Record{
			mkRecord(&directory.Group{Id: "group1", Name: "group 1", Email: "group1@example.com"}),
			mkRecord(&directory.User{Id: "user1", DisplayName: "user 1", Email: "user1@example.com", GroupIds: []string{"group1s"}}),
			mkRecord(&session.Session{Id: "session1", UserId: "user1"}),
			mkRecord(&user.User{Id: "user1", Name: "user 1", Email: "user1@example.com"}),
		},
	})

	assert.NotNil(t, mgr.directoryGroups["group1"])
	assert.NotNil(t, mgr.directoryUsers["user1"])
	if _, ok := mgr.sessions.Get("user1", "session1"); assert.True(t, ok) {

	}
	if _, ok := mgr.users.Get("user1"); assert.True(t, ok) {
		tm, id := mgr.userScheduler.Next()
		assert.Equal(t, now.Add(time.Hour), tm)
		assert.Equal(t, "user1", id)
	}

}

func TestManager_refreshDirectoryUserGroups(t *testing.T) {
	ctx, clearTimeout := context.WithTimeout(context.Background(), time.Second*10)
	defer clearTimeout()

	t.Run("backoff", func(t *testing.T) {
		cnt := 0
		mgr := New(
			WithDirectoryProvider(mockProvider{
				userGroups: func(ctx context.Context) ([]*directory.Group, []*directory.User, error) {
					cnt++
					switch cnt {
					case 1:
						return nil, nil, fmt.Errorf("error 1")
					case 2:
						return nil, nil, fmt.Errorf("error 2")
					}
					return nil, nil, nil
				},
			}),
			WithGroupRefreshInterval(time.Hour),
		)
		mgr.directoryBackoff.RandomizationFactor = 0 // disable randomization for deterministic testing

		dur1 := mgr.refreshDirectoryUserGroups(ctx)
		dur2 := mgr.refreshDirectoryUserGroups(ctx)
		dur3 := mgr.refreshDirectoryUserGroups(ctx)

		assert.Greater(t, dur2, dur1)
		assert.Greater(t, dur3, dur2)
		assert.Equal(t, time.Hour, dur3)
	})
}

func mkRecord(msg recordable) *databroker.Record {
	any := protoutil.NewAny(msg)
	return &databroker.Record{
		Type: any.GetTypeUrl(),
		Id:   msg.GetId(),
		Data: any,
	}
}

type recordable interface {
	proto.Message
	GetId() string
}
