package grpcmw_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	goratelimit "github.com/krishna-kudari/ratelimit"
	"github.com/krishna-kudari/ratelimit/middleware/grpcmw"

	testgrpc "google.golang.org/grpc/interop/grpc_testing"
)

// ─── Test Service ────────────────────────────────────────────────────────────

type testServer struct {
	testgrpc.UnimplementedTestServiceServer
}

func (s *testServer) EmptyCall(_ context.Context, _ *testgrpc.Empty) (*testgrpc.Empty, error) {
	return &testgrpc.Empty{}, nil
}

func (s *testServer) UnaryCall(_ context.Context, req *testgrpc.SimpleRequest) (*testgrpc.SimpleResponse, error) {
	return &testgrpc.SimpleResponse{}, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func startServer(t *testing.T, opts ...grpc.ServerOption) (testgrpc.TestServiceClient, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer(opts...)
	testgrpc.RegisterTestServiceServer(srv, &testServer{})

	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		srv.Stop()
	}
	require.NoError(t, err)

	client := testgrpc.NewTestServiceClient(conn)
	cleanup := func() {
		conn.Close()
		srv.Stop()
	}
	return client, cleanup
}

// ─── Unary Tests ─────────────────────────────────────────────────────────────

func TestUnaryServerInterceptor_AllowsWithinLimit(t *testing.T) {
	limiter, err := goratelimit.NewFixedWindow(5, 60)
	require.NoError(t, err)

	client, cleanup := startServer(t,
		grpc.ChainUnaryInterceptor(grpcmw.UnaryServerInterceptor(limiter, grpcmw.KeyByPeer)),
	)
	defer cleanup()

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		var header metadata.MD
		_, err := client.EmptyCall(ctx, &testgrpc.Empty{}, grpc.Header(&header))
		require.NoError(t, err, "request %d: unexpected error", i+1)

		limit := header.Get("x-ratelimit-limit")
		require.NotEmpty(t, limit, "request %d: expected x-ratelimit-limit", i+1)
		assert.Equal(t, "5", limit[0], "request %d: expected x-ratelimit-limit=5", i+1)
	}
}

func TestUnaryServerInterceptor_DeniesExceedingLimit(t *testing.T) {
	limiter, err := goratelimit.NewFixedWindow(3, 60)
	require.NoError(t, err)

	client, cleanup := startServer(t,
		grpc.ChainUnaryInterceptor(grpcmw.UnaryServerInterceptor(limiter, grpcmw.KeyByPeer)),
	)
	defer cleanup()

	ctx := context.Background()

	// Exhaust limit
	for i := 0; i < 3; i++ {
		_, err := client.EmptyCall(ctx, &testgrpc.Empty{})
		require.NoError(t, err, "request %d should be allowed", i+1)
	}

	// 4th request should be denied
	_, err = client.EmptyCall(ctx, &testgrpc.Empty{})
	require.Error(t, err, "expected error on 4th request")
	st, ok := status.FromError(err)
	require.True(t, ok, "expected gRPC status error, got %v", err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestUnaryServerInterceptor_RateLimitHeaders(t *testing.T) {
	limiter, err := goratelimit.NewFixedWindow(10, 60)
	require.NoError(t, err)

	client, cleanup := startServer(t,
		grpc.ChainUnaryInterceptor(grpcmw.UnaryServerInterceptor(limiter, grpcmw.KeyByPeer)),
	)
	defer cleanup()

	var header metadata.MD
	_, err = client.EmptyCall(context.Background(), &testgrpc.Empty{}, grpc.Header(&header))
	require.NoError(t, err)

	for _, key := range []string{"x-ratelimit-limit", "x-ratelimit-remaining", "x-ratelimit-reset"} {
		assert.NotEmpty(t, header.Get(key), "expected %s header in response metadata", key)
	}
}

func TestUnaryServerInterceptor_HeadersDisabled(t *testing.T) {
	limiter, err := goratelimit.NewFixedWindow(10, 60)
	require.NoError(t, err)

	noHeaders := false
	client, cleanup := startServer(t,
		grpc.ChainUnaryInterceptor(grpcmw.UnaryServerInterceptorWithConfig(grpcmw.Config{
			Limiter: limiter,
			KeyFunc: grpcmw.KeyByPeer,
			Headers: &noHeaders,
		})),
	)
	defer cleanup()

	var header metadata.MD
	_, err = client.EmptyCall(context.Background(), &testgrpc.Empty{}, grpc.Header(&header))
	require.NoError(t, err)

	assert.Empty(t, header.Get("x-ratelimit-limit"), "headers should not be set when disabled")
}

func TestUnaryServerInterceptor_ExcludeMethods(t *testing.T) {
	limiter, err := goratelimit.NewFixedWindow(1, 60)
	require.NoError(t, err)

	client, cleanup := startServer(t,
		grpc.ChainUnaryInterceptor(grpcmw.UnaryServerInterceptorWithConfig(grpcmw.Config{
			Limiter: limiter,
			KeyFunc: grpcmw.KeyByPeer,
			ExcludeMethods: map[string]bool{
				"/grpc.testing.TestService/EmptyCall": true,
			},
		})),
	)
	defer cleanup()

	ctx := context.Background()

	// EmptyCall is excluded — should always succeed
	for i := 0; i < 5; i++ {
		_, err := client.EmptyCall(ctx, &testgrpc.Empty{})
		require.NoError(t, err, "excluded method should not be rate limited, request %d", i+1)
	}
}

func TestUnaryServerInterceptor_CustomDeniedHandler(t *testing.T) {
	limiter, err := goratelimit.NewFixedWindow(1, 60)
	require.NoError(t, err)

	customCalled := false
	client, cleanup := startServer(t,
		grpc.ChainUnaryInterceptor(grpcmw.UnaryServerInterceptorWithConfig(grpcmw.Config{
			Limiter: limiter,
			KeyFunc: grpcmw.KeyByPeer,
			DeniedHandler: func(_ context.Context, result *goratelimit.Result) error {
				customCalled = true
				return status.Errorf(codes.Unavailable, "custom: throttled for %v", result.RetryAfter)
			},
		})),
	)
	defer cleanup()

	ctx := context.Background()

	// Exhaust
	_, _ = client.EmptyCall(ctx, &testgrpc.Empty{})

	// Trigger denial
	_, err = client.EmptyCall(ctx, &testgrpc.Empty{})
	require.Error(t, err, "expected denial")
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code(), "expected Unavailable from custom handler")
	// customCalled is set in the server goroutine; give it a moment
	time.Sleep(10 * time.Millisecond)
	assert.True(t, customCalled, "custom denied handler should have been called")
}

func TestUnaryServerInterceptor_KeyByMetadata(t *testing.T) {
	limiter, err := goratelimit.NewFixedWindow(2, 60)
	require.NoError(t, err)

	client, cleanup := startServer(t,
		grpc.ChainUnaryInterceptor(grpcmw.UnaryServerInterceptor(limiter, grpcmw.KeyByMetadata("x-api-key"))),
	)
	defer cleanup()

	// key-A: exhaust 2 requests
	ctxA := metadata.AppendToOutgoingContext(context.Background(), "x-api-key", "key-A")
	for i := 0; i < 2; i++ {
		_, err := client.EmptyCall(ctxA, &testgrpc.Empty{})
		require.NoError(t, err, "key-A request %d should succeed", i+1)
	}

	// key-A: 3rd request should be denied
	_, err = client.EmptyCall(ctxA, &testgrpc.Empty{})
	require.Error(t, err, "key-A 3rd request should be denied")

	// key-B: should still be allowed (separate key)
	ctxB := metadata.AppendToOutgoingContext(context.Background(), "x-api-key", "key-B")
	_, err = client.EmptyCall(ctxB, &testgrpc.Empty{})
	require.NoError(t, err, "key-B should be allowed")
}

func TestUnaryServerInterceptor_KeyByMethod(t *testing.T) {
	limiter, err := goratelimit.NewFixedWindow(1, 60)
	require.NoError(t, err)

	client, cleanup := startServer(t,
		grpc.ChainUnaryInterceptor(grpcmw.UnaryServerInterceptor(limiter, grpcmw.KeyByMethod)),
	)
	defer cleanup()

	ctx := context.Background()

	// EmptyCall — use up its 1 allowed request
	_, err = client.EmptyCall(ctx, &testgrpc.Empty{})
	require.NoError(t, err)

	// EmptyCall — should be denied
	_, err = client.EmptyCall(ctx, &testgrpc.Empty{})
	require.Error(t, err, "2nd EmptyCall should be denied")

	// UnaryCall — different method, should succeed
	_, err = client.UnaryCall(ctx, &testgrpc.SimpleRequest{})
	require.NoError(t, err, "UnaryCall should be allowed (different method key)")
}

func TestUnaryServerInterceptor_DifferentAlgorithms(t *testing.T) {
	algorithms := []struct {
		name    string
		limiter goratelimit.Limiter
	}{
		{"GCRA", mustLimiter(goratelimit.NewGCRA(100, 3))},
		{"TokenBucket", mustLimiter(goratelimit.NewTokenBucket(3, 1))},
		{"FixedWindow", mustLimiter(goratelimit.NewFixedWindow(3, 60))},
		{"SlidingWindowCounter", mustLimiter(goratelimit.NewSlidingWindowCounter(3, 60))},
	}

	for _, alg := range algorithms {
		t.Run(alg.name, func(t *testing.T) {
			client, cleanup := startServer(t,
				grpc.ChainUnaryInterceptor(grpcmw.UnaryServerInterceptor(alg.limiter, grpcmw.KeyByPeer)),
			)
			defer cleanup()

			ctx := context.Background()
			for i := 0; i < 3; i++ {
				_, err := client.EmptyCall(ctx, &testgrpc.Empty{})
				require.NoError(t, err, "%s: request %d should be allowed", alg.name, i+1)
			}

			_, err := client.EmptyCall(ctx, &testgrpc.Empty{})
			assert.Error(t, err, "%s: 4th request should be denied", alg.name)
		})
	}
}

func mustLimiter(l goratelimit.Limiter, err error) goratelimit.Limiter {
	if err != nil {
		panic(err)
	}
	return l
}
