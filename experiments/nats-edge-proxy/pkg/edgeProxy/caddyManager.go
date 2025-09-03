package edgeProxy

import (
	"math/rand"
	"net/http"
	"net/http/httputil"
	"strings"
)

func (ep *EdgeProxy) StartServer() {
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		// find the route from our service map
		var routeFound Route

		ep.routesMu.RLock()
		for _, route := range ep.routes {
			if strings.ToLower(route.VHost) == strings.ToLower(request.Host) {
				routeFound = route
				break
			}
		}
		ep.routesMu.RUnlock()

		if routeFound.Endpoint == nil {
			writer.WriteHeader(404)
			writer.Write([]byte("404 host not found."))
			return
		}

		if len(routeFound.Endpoint) == 0 {
			writer.WriteHeader(http.StatusBadGateway)
			writer.Write([]byte("no healthy upstream."))
			return
		}

		chosenEndpoint := routeFound.Endpoint[rand.Intn(len(routeFound.Endpoint))]
		httputil.NewSingleHostReverseProxy(chosenEndpoint).ServeHTTP(writer, request)
	})
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}
}
