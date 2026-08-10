What is a Service Mesh?

Linkerd vs Istio? What's an envoy?

What is a Reverse Proxy?

Why use Service Mesh? 2 Reasons: 1 - decouple operational logic from the business application code. 2 - provide reliability, security and observability logic for your microservice setup

Use dependency injection for scalability: for eg: Attach your dependencies to a struct and make `Home` a method on that struct, the (s *Server) part so that Proxy is reachable inside that method

 While Proxying, need to pass Original Headers - why? The `httputil.NewSingleHostReverseProxy` in Go does not automatically update the `Host` header of the incoming request. Because of this, your proxy sends your local host header (e.g., `localhost:8080`) to Google. Google sees an unrecognized host and returns a 404 error.
