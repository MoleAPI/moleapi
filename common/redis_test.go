package common

import (
	"context"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	miniredisServer "github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitRedisClientConnectionModes(t *testing.T) {
	previousRDB, previousEnabled := RDB, RedisEnabled
	t.Cleanup(func() {
		if RDB != nil && RDB != previousRDB {
			_ = RDB.Close()
		}
		RDB, RedisEnabled = previousRDB, previousEnabled
	})

	t.Run("direct connection remains compatible", func(t *testing.T) {
		server := miniredis.RunT(t)
		server.RequireAuth("data-password")
		t.Setenv("REDIS_CONN_STRING", fmt.Sprintf("redis://:data-password@%s/2", server.Addr()))
		t.Setenv("REDIS_POOL_SIZE", "7")
		t.Setenv("REDIS_SENTINEL_MASTER_NAME", "")
		t.Setenv("REDIS_SENTINEL_ADDRS", "")

		require.NoError(t, InitRedisClient())
		require.NoError(t, RDB.Set(context.Background(), "direct", "ok", 0).Err())
		value, err := server.DB(2).Get("direct")
		require.NoError(t, err)
		assert.Equal(t, "ok", value)
		assert.Equal(t, 7, RDB.Options().PoolSize)
	})

	t.Run("sentinel discovers the master", func(t *testing.T) {
		master := miniredis.RunT(t)
		master.RequireAuth("data-password")
		sentinel := miniredis.RunT(t)
		sentinel.RequireAuth("sentinel-password")
		require.NoError(t, sentinel.Server().Register("sentinel", func(peer *miniredisServer.Peer, _ string, args []string) {
			if len(args) != 2 || args[0] != "get-master-addr-by-name" || args[1] != "mole-test" {
				peer.WriteError("unexpected sentinel command")
				return
			}
			peer.WriteLen(2)
			peer.WriteBulk(master.Host())
			peer.WriteBulk(master.Port())
		}))

		t.Setenv("REDIS_CONN_STRING", "redis://:data-password@unused.invalid:6379/3")
		t.Setenv("REDIS_POOL_SIZE", "9")
		t.Setenv("REDIS_SENTINEL_MASTER_NAME", "mole-test")
		t.Setenv("REDIS_SENTINEL_ADDRS", " , "+sentinel.Addr()+", ")
		t.Setenv("REDIS_SENTINEL_PASSWORD", "sentinel-password")

		require.NoError(t, InitRedisClient())
		require.NoError(t, RDB.Set(context.Background(), "sentinel", "ok", 0).Err())
		value, err := master.DB(3).Get("sentinel")
		require.NoError(t, err)
		assert.Equal(t, "ok", value)
		assert.Equal(t, 9, RDB.Options().PoolSize)
	})
}
