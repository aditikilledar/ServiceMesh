package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	pb "servicemesh/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	var waitgroup sync.WaitGroup

	cp := NewControlPlane()

	// 1. Start Control Plane gRPC Server
	go func() {
		if err := StartControlPlaneServer(":50051", cp); err != nil {
			log.Fatalf("Control Plane gRPC server failed: %v", err)
		}
	}()

	waitForPort("127.0.0.1:50051", 3*time.Second)

	// 2. Connect gRPC Client
	conn, err := grpc.NewClient("127.0.0.1:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Could not connect to control plane: %v", err)
	}
	defer conn.Close()
	grpcClient := pb.NewControlPlaneClient(conn)

	// 3. Start Backend App Server - with multiple instances (cus we need to demo load balancing)
	multiplePorts := []string{"127.0.0.1:8081", "127.0.0.1:8082", "127.0.0.1:8083"}
	for _, port := range multiplePorts {
		app := &ApplicationServer{
			listenPort: port,
		}
		waitgroup.Add(1)
		// start the application
		go func(a *ApplicationServer) {
			if err := a.StartAppServer(&waitgroup); err != nil {
				log.Fatal("Error starting Application on port: ", port, "with error: ", err)
			}
		}(app)
	}

	// wait for all the app servers to start
	waitgroup.Wait()
	log.Printf("Application Servers running on %s.", multiplePorts)

	// 4. Initialize Sidecar
	sidecarProxy := NewSidecar("user-service", ":8080", grpcClient)

	// FIRST: make sidecar register its subscriber channel with the publisher (aka route stream), to avoid a case where pub is sending updates but nobody is subscribed
	ctxStream := context.Background()
	sidecarProxy.StartRouteStream(ctxStream, "user-service")
	time.Sleep(100 * time.Millisecond)

	// 5. Register with Control Plane FIRST - for all of the applications
	for _, port := range multiplePorts {
		ctxRegistration, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		addr := fmt.Sprintf("http://%s", port)

		log.Printf("> Dynamically registering instance: %s", addr)
		if err := sidecarProxy.RegisterServiceWithControlPlane(ctxRegistration, addr); err != nil {
			log.Printf("Failed to register %s: %v", addr, err)
		}
		cancel()

		// making it sleep so it's not too fast lol, so us puny humans can observe the changes
		time.Sleep(2 * time.Second)

	}

	// 6. Start HTTP Proxy (blocking), nil waitgroup because we don't want to wait for anything while starting this server
	if err := sidecarProxy.StartSidecar(nil); err != nil {
		log.Fatal(err)
	}
}

func waitForPort(addr string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}
