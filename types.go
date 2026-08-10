package main

import (
	"net/http"
	"net/http/httputil"
)

type ApplicationServer struct {
	listenPort string // the port num the app server starts on/listens on
	router     *http.ServeMux
}

type Sidecar struct {
	targetUrl  string
	listenPort string // the port num the proxy server starts on/listens on
	proxy      *httputil.ReverseProxy
	controller *ControlPlane
}

type ControlPlane struct {
	routes map[string]string
}
