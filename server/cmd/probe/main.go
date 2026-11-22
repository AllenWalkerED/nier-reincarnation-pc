// probe makes the client's first gRPC calls (config, register, auth) against a
// Lunar Tear server, to check the server end of the chain without the game.
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "lunar-tear/server/gen/proto"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8003", "gRPC server address")
	flag.Parse()
	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cfg, err := pb.NewConfigServiceClient(conn).GetReviewServerConfig(ctx, &emptypb.Empty{})
	if err != nil {
		log.Fatalf("ConfigService.GetReviewServerConfig: %v", err)
	}
	log.Printf("ConfigService.GetReviewServerConfig OK: %v", cfg)

	users := pb.NewUserServiceClient(conn)
	id := uuid.NewString()
	reg, err := users.RegisterUser(ctx, &pb.RegisterUserRequest{Uuid: id, TerminalId: "probe"})
	if err != nil {
		log.Fatalf("UserService.RegisterUser: %v", err)
	}
	log.Printf("UserService.RegisterUser OK: userId=%d", reg.UserId)

	auth, err := users.Auth(ctx, &pb.AuthUserRequest{Uuid: id, Signature: reg.Signature})
	if err != nil {
		log.Fatalf("UserService.Auth: %v", err)
	}
	log.Printf("UserService.Auth OK: %v", auth)
}
