// Package middleware provides rate limiting middleware for HTTP and gRPC servers.
//
// # gRPC Interceptors
//
// gRPC interceptors are not included directly to avoid adding google.golang.org/grpc
// as a mandatory dependency. Use the patterns below to integrate with gRPC.
//
// Unary server interceptor:
//
//	import (
//	    "context"
//	    goratelimit "github.com/krishna-kudari/ratelimit"
//	    "google.golang.org/grpc"
//	    "google.golang.org/grpc/codes"
//	    "google.golang.org/grpc/metadata"
//	    "google.golang.org/grpc/peer"
//	    "google.golang.org/grpc/status"
//	)
//
//	func RateLimitUnaryInterceptor(limiter goratelimit.Limiter, keyFunc func(ctx context.Context) string) grpc.UnaryServerInterceptor {
//	    return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
//	        key := keyFunc(ctx)
//	        result, err := limiter.Allow(ctx, key)
//	        if err != nil {
//	            return handler(ctx, req) // fail open
//	        }
//	        if !result.Allowed {
//	            return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded, retry after %v", result.RetryAfter)
//	        }
//	        return handler(ctx, req)
//	    }
//	}
//
// Key extractor using peer address:
//
//	func KeyByPeer(ctx context.Context) string {
//	    p, ok := peer.FromContext(ctx)
//	    if ok {
//	        return p.Addr.String()
//	    }
//	    return "unknown"
//	}
//
// Key extractor using metadata:
//
//	func KeyByMetadata(header string) func(ctx context.Context) string {
//	    return func(ctx context.Context) string {
//	        md, ok := metadata.FromIncomingContext(ctx)
//	        if ok {
//	            if vals := md.Get(header); len(vals) > 0 {
//	                return vals[0]
//	            }
//	        }
//	        return "unknown"
//	    }
//	}
//
// Server setup:
//
//	limiter, _ := goratelimit.NewGCRA(1000, 50, goratelimit.WithRedis(redisClient))
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(RateLimitUnaryInterceptor(limiter, KeyByPeer)),
//	)
package middleware
