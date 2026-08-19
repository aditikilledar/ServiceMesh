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
	serviceName string
	listenPort  string // the port num the proxy server starts on/listens on
	proxy       *httputil.ReverseProxy
	grpcClient  pb.ControlPlaneClient

	// local endpoint cache, load balancer state
	cacheMu   sync.RWMutex
	lbIndices map[string]*uint64  // serviceName -> atomic roundrobin counter
	endpoints map[string][]string // serviceName -> slice/list of active instance URLs for the service
}

type ControlPlane struct {
	routes map[string]string
	mu     sync.RWMutex
}

// ControlPlaneServer wraps BOTH the gRPC safety net and your custom state
type ControlPlaneServer struct {
	pb.UnimplementedControlPlaneServer // Embeds default gRPC behavior - needed to cover unimplmented new methods and all that
	cp                                 *ControlPlane
}
