package main

import (
	"net/http"
	"net/http/httputil"
	"sync"

	pb "servicemesh/proto"
)

type ApplicationServer struct {
	listenPort string // the port num the app server starts on/listens on
	router     *http.ServeMux
}

type Sidecar struct {
	targetUrl   string
	listenPort  string // the port num the proxy server starts on/listens on
	proxy       *httputil.ReverseProxy
	grpcClient  pb.ControlPlaneServiceClient
	serviceName string
}

type ControlPlane struct {
	routes map[string]string
	mu     sync.RWMutex
}

// ControlPlaneServer wraps BOTH the gRPC safety net and your custom state
type ControlPlaneServer struct {
	pb.UnimplementedControlPlaneServiceServer // Embeds default gRPC behavior - needed to cover unimplmented new methods and all that
	cp                                        *ControlPlane
}
