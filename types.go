package main

import "net/http/httputil"

type Server struct {
	revProxy   *httputil.ReverseProxy
	portNumber string
}
