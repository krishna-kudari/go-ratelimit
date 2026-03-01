// gRPC server with rate limiting interceptors.
// Run: go run ./examples/grpcserver/
//
// Wire the interceptors into your existing gRPC server.
// Both unary and streaming RPCs are supported.
package main

import (
	"log"
	"net"

	goratelimit "github.com/krishna-kudari/ratelimit"
	"github.com/krishna-kudari/ratelimit/middleware/grpcmw"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	limiter, _ := goratelimit.NewTokenBucket(100, 10)

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpcmw.UnaryServerInterceptor(limiter, grpcmw.KeyByPeer),
		),
		grpc.ChainStreamInterceptor(
			grpcmw.StreamServerInterceptor(limiter, grpcmw.StreamKeyByPeer),
		),
	)

	// Register your services — using health check as a demo service
	healthpb.RegisterHealthServer(server, health.NewServer())

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatal(err)
	}
	log.Println("gRPC server listening on :50051")
	log.Fatal(server.Serve(lis))
}
