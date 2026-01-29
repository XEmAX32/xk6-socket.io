package socketio

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildSocketIOWSURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		host      string
		opts      Options
		wantScheme string
		wantHost  string
		wantPath  string
		wantQuery url.Values
		wantErr   bool
	}{
		{
			name:      "http to ws with custom query",
			host:      "http://example.com",
			opts:      Options{Path: "socket.io", Query: map[string]any{"foo": "bar"}},
			wantScheme: "ws",
			wantHost:  "example.com",
			wantPath:  "/socket.io",
			wantQuery: url.Values{"EIO": {"4"}, "transport": {"websocket"}, "foo": {"bar"}},
		},
		{
			name:      "https to wss with leading slash",
			host:      "https://example.com:443",
			opts:      Options{Path: "/socket.io/"},
			wantScheme: "wss",
			wantHost:  "example.com:443",
			wantPath:  "/socket.io/",
			wantQuery: url.Values{"EIO": {"4"}, "transport": {"websocket"}},
		},
		{
			name:    "unsupported scheme",
			host:    "ftp://example.com",
			opts:    Options{Path: "/socket.io/"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			urlStr, err := buildSocketIOWSURL(tt.host, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			parsed, err := url.Parse(urlStr)
			require.NoError(t, err)
			require.Equal(t, tt.wantScheme, parsed.Scheme)
			require.Equal(t, tt.wantHost, parsed.Host)
			require.Equal(t, tt.wantPath, parsed.Path)

			gotQuery := parsed.Query()
			for key, vals := range tt.wantQuery {
				require.Equal(t, vals, gotQuery[key])
			}
		})
	}
}

func TestExtractEvent(t *testing.T) {
	t.Parallel()

	event, data, err := extractEvent(`["hello", {"msg": "hi"}]`)
	require.NoError(t, err)
	require.Equal(t, "hello", event)
	require.Equal(t, []any{map[string]any{"msg": "hi"}}, data)

	event, data, err = extractEvent(`["user_updated", {"id": 1}, {"name": "Alice"}]`)
	require.NoError(t, err)
	require.Equal(t, "user_updated", event)
	require.Equal(t, []any{
		map[string]any{"id": float64(1)},
		map[string]any{"name": "Alice"},
	}, data)

	event, data, err = extractEvent(`["ping"]`)
	require.NoError(t, err)
	require.Equal(t, "ping", event)
	require.Nil(t, data)

	_, _, err = extractEvent(`[1]`)
	require.Error(t, err)

	_, _, err = extractEvent(`["bad"`)
	require.Error(t, err)
}
