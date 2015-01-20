package main

import (
	"flag"
	. "github.com/zettio/weave/common"
	"io"
	"net"
	"net/http"
	"net/url"
)

const (
	listenOn = ":12375"
)

type Proxy struct {
	Transport http.RoundTripper
}

func targetNetwork(u *url.URL) string {
	return u.Scheme
}

func targetAddress(u *url.URL) (addr string) {
	switch u.Scheme {
	case "tcp":
		addr = u.Host
	case "unix":
		addr = u.Path
	}
	return
}

func makeProxy(targetUrl string) (*Proxy, error) {
	u, err := url.Parse(targetUrl)
	if err != nil {
		return nil, err
	}
	return &Proxy{
		Transport: &http.Transport{
			Proxy: func(_ *http.Request) (*url.URL, error) { return nil, nil },
			Dial: func(_network, _address string) (net.Conn, error) {
				return net.Dial(targetNetwork(u), targetAddress(u))
			},
		},
	}, nil
}

func main() {
	var target, listen string

	flag.StringVar(&target, "H", "unix:///var/run/docker.sock", "docker daemon URL to proxy")
	flag.StringVar(&listen, "L", ":12375", "address on which to listen")
	flag.Parse()

	p, err := makeProxy(target)
	s := &http.Server{
		Addr:    listen,
		Handler: p,
	}

	Info.Printf("Listening on %s", listen)
	Info.Printf("Proxying %s", target)

	err = s.ListenAndServe()
	if err != nil {
		Error.Fatalf("Could not listen on %s: %s", listen, err)
	}
}

func (proxy *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Info.Printf("%s %s", r.Method, r.URL)
	// These just to fool the HTTP code into doing my bidding
	r.RequestURI = ""
	r.URL.Scheme = "http"
	r.URL.Host = "localhost"

	resp, err := proxy.Transport.RoundTrip(r)
	if err != nil {
		Warning.Printf("Failed to proxy %s %s: %s", r.Method, r.URL.Path, err)
		w.WriteHeader(500)
		return
	}

	hdr := w.Header()
	for k, vs := range resp.Header {
		for _, v := range vs {
			hdr.Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		Warning.Print(err)
	}
}
