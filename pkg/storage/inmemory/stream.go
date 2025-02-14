package inmemory

import (
	"context"

	"github.com/pomerium/pomerium/pkg/grpc/databroker"
	"github.com/pomerium/pomerium/pkg/storage"
)

func newSyncLatestRecordStream(
	ctx context.Context,
	backend *Backend,
) storage.RecordStream {
	var ready []*databroker.Record
	return storage.NewRecordStream(ctx, backend.closed, []storage.RecordStreamGenerator{
		func(ctx context.Context, block bool) (*databroker.Record, error) {
			backend.mu.RLock()
			for _, co := range backend.lookup {
				ready = append(ready, co.List()...)
			}
			backend.mu.RUnlock()
			return nil, storage.ErrStreamDone
		},
		func(ctx context.Context, block bool) (*databroker.Record, error) {
			if len(ready) == 0 {
				return nil, storage.ErrStreamDone
			}

			record := ready[0]
			ready = ready[1:]
			return dup(record), nil
		},
	}, nil)
}

func newSyncRecordStream(
	ctx context.Context,
	backend *Backend,
	recordVersion uint64,
) storage.RecordStream {
	changed := backend.onChange.Bind()
	var ready []*databroker.Record
	return storage.NewRecordStream(ctx, backend.closed, []storage.RecordStreamGenerator{
		func(ctx context.Context, block bool) (*databroker.Record, error) {
			if len(ready) > 0 {
				record := ready[0]
				ready = ready[1:]
				return record, nil
			}

			for {
				ready = backend.getSince(recordVersion)

				if len(ready) > 0 {
					// records are sorted by version,
					// so update the local version to the last record
					recordVersion = ready[len(ready)-1].GetVersion()
					record := ready[0]
					ready = ready[1:]
					return record, nil
				} else if !block {
					return nil, storage.ErrStreamDone
				}

				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-changed:
				}
			}
		},
	}, func() {
		backend.onChange.Unbind(changed)
	})
}
