package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	captchapb "github.com/Pupervemon/risk-proto/gen/go/captcha/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	addr := flag.String("addr", "localhost:9091", "captcha gRPC server address")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Usage: go run test_grpc_client.go [-addr host:port] <TOKEN>")
		return
	}
	token := flag.Arg(0)

	conn, err := grpc.Dial(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := captchapb.NewCaptchaTokenServiceClient(conn)

	// Call VerifyToken
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	resp, err := client.VerifyToken(ctx, &captchapb.VerifyTokenRequest{
		Token: token,
	})
	if err != nil {
		log.Fatalf("Failed to call VerifyToken: %v", err)
	}

	fmt.Println("--- gRPC Verification Result ---")
	fmt.Printf("Valid: %v\n", resp.Valid)
	fmt.Printf("Reason: %s\n", resp.Reason)
	if resp.ExpiresAt > 0 {
		fmt.Printf("Expires At: %v\n", time.Unix(resp.ExpiresAt, 0).Format("2006-01-02 15:04:05"))
	} else {
		fmt.Printf("Expires At: N/A\n")
	}
}
