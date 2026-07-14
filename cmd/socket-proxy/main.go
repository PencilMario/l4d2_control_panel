package main

import (
	"context"
	"github.com/not0721here/l4d2-control-panel/internal/socketproxy"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"
)

func main() {
	socket := os.Getenv("DOCKER_SOCKET")
	if socket == "" {
		socket = "/var/run/docker.sock"
	}
	target, _ := url.Parse("http://docker")
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socket)
	}}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !socketproxy.Allowed(r.Method, r.URL.Path) {
			http.Error(w, "docker endpoint forbidden", http.StatusForbidden)
			return
		}
		proxy.ServeHTTP(w, r)
	})
	server := &http.Server{Addr: ":2375", Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	log.Print("restricted Docker proxy listening on :2375")
	log.Fatal(server.ListenAndServe())
}
