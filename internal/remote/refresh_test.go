package remote

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogRefreshKeepsHealthyPeerCurrentWhenOthersTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := testConfig()
		for i := range 24 {
			id := fmt.Sprintf("slow%d", i)
			cfg.Peers = append(cfg.Peers, Peer{ID: id, SSH: id})
		}
		s := NewServer(cfg, &fakeClient{})
		var slow atomic.Bool
		s.PeerCall = func(ctx context.Context, peer Peer, _ Request) (Response, error) {
			if slow.Load() && peer.ID != "other" {
				<-ctx.Done()
				return Response{}, ctx.Err()
			}
			return Response{Catalog: []Machine{{ID: peer.ID, Session: peer.Session}}}, nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { s.refreshCatalog(ctx); close(done) }()
		defer func() { cancel(); <-done }()
		synctest.Wait()
		initial := s.Handle(ctx, Request{Op: "list"}).Catalog
		require.False(t, initial[1].Stale)
		slow.Store(true)
		for range 30 {
			time.Sleep(time.Second)
			synctest.Wait()
			catalog := s.Handle(ctx, Request{Op: "list"}).Catalog
			require.False(t, catalog[0].Stale)
			require.False(t, catalog[1].Stale, "unrelated peers must not expire a healthy catalog")
			require.LessOrEqual(t, time.Since(catalog[1].UpdatedAt), 5*time.Second, "healthy peer must keep its refresh cadence")
		}
		assert.True(t, s.Handle(ctx, Request{Op: "list"}).Catalog[2].Stale)
	})
}
