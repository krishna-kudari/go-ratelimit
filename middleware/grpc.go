// This file is kept for backward-compatibility documentation.
// The concrete gRPC interceptor implementation lives in the grpcmw sub-package
// to avoid pulling google.golang.org/grpc into projects that only need HTTP middleware.
//
// Import:
//
//	import "github.com/krishna-kudari/ratelimit/middleware/grpcmw"
//
// Unary interceptor:
//
//	limiter, _ := goratelimit.NewGCRA(1000, 50, goratelimit.WithRedis(redisClient))
//	server := grpc.NewServer(
//	    grpc.ChainUnaryInterceptor(grpcmw.UnaryServerInterceptor(limiter, grpcmw.KeyByPeer)),
//	)
//
// Stream interceptor:
//
//	server := grpc.NewServer(
//	    grpc.ChainStreamInterceptor(grpcmw.StreamServerInterceptor(limiter, grpcmw.StreamKeyByPeer)),
//	)
//
// Key extractors:
//
//	grpcmw.KeyByPeer           — remote peer address
//	grpcmw.KeyByMetadata("x-api-key") — value from gRPC metadata
//	grpcmw.KeyByMethod         — full method + peer (per-endpoint limiting)
//
// Full config:
//
//	grpcmw.UnaryServerInterceptorWithConfig(grpcmw.Config{
//	    Limiter:        limiter,
//	    KeyFunc:        grpcmw.KeyByPeer,
//	    ExcludeMethods: map[string]bool{"/pkg.Service/Health": true},
//	    DeniedHandler:  customHandler,
//	})
//
// See package github.com/krishna-kudari/ratelimit/middleware/grpcmw for full API.
package middleware
