package main

import (
	"flag"
	. "github.com/zettio/weave/common"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
)

const (
	RAW_STREAM = "application/vnd.docker.raw-stream"
)

type Proxy struct {
	Dial func() (net.Conn, error)
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
		Dial: func() (net.Conn, error) {
			return net.Dial(targetNetwork(u), targetAddress(u))
		},
	}, nil
}

func main() {
	var target, listen string
	var debug bool

	flag.StringVar(&target, "H", "unix:///var/run/docker.sock", "docker daemon URL to proxy")
	flag.StringVar(&listen, "L", ":12375", "address on which to listen")
	flag.BoolVar(&debug, "debug", false, "log debugging information")
	flag.Parse()

	if debug {
		InitDefaultLogging(true)
	}

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
	req, err := http.NewRequest(r.Method, r.URL.Path, r.Body)
	if err != nil {
		http.Error(w, "Unable to create proxied request", http.StatusInternalServerError)
		Warning.Print(err)
		return
	}
	req.Close = false
	req.Header = r.Header
	req.URL.RawQuery = r.URL.RawQuery

	conn, err := proxy.Dial()
	if err != nil {
		http.Error(w, "Could not connect to target", http.StatusInternalServerError)
		Warning.Print(err)
		return
	}
	client := httputil.NewClientConn(conn, nil)
	defer client.Close()

	resp, _ := client.Do(req)

	hdr := w.Header()
	for k, vs := range resp.Header {
		for _, v := range vs {
			hdr.Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	Debug.Printf("Response from target: %s %+v", resp.Status, w.Header())

	if resp.Header.Get("Content-Type") == RAW_STREAM ||
		(resp.TransferEncoding != nil &&
			resp.TransferEncoding[0] == "chunked") {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "Unable to use raw stream mode", http.StatusInternalServerError)
			return
		}

		down, _, err := hj.Hijack()
		if err != nil {
			http.Error(w, "Unable to switch to raw stream mode", http.StatusInternalServerError)
			return
		}
		defer down.Close()

		up, _ := client.Hijack()
		defer up.Close()

		end := make(chan bool)

		go func() {
			defer close(end)
			_, err := io.Copy(down, up)
			if err != nil {
				Warning.Print(err)
			}
		}()
		go func() {
			_, err := io.Copy(up, down)
			if err != nil {
				Warning.Print(err)
			}
			up.(interface {
				CloseWrite() error
			}).CloseWrite()
		}()
		<-end
	} else {
		_, err = io.Copy(w, resp.Body)
		if err != nil {
			Warning.Print(err)
		}
	}
}
