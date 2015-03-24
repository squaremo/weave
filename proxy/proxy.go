package proxy

import (
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

func NewProxy(targetUrl string) (*Proxy, error) {
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

func (proxy *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Info.Printf("%s %s", r.Method, r.URL)
	req, err := proxy.InterceptRequest(r)
	if err != nil {
		http.Error(w, "Unable to create proxied request", http.StatusInternalServerError)
		Warning.Print(err)
		return
	}

	conn, err := proxy.Dial()
	if err != nil {
		http.Error(w, "Could not connect to target", http.StatusInternalServerError)
		Warning.Print(err)
		return
	}
	client := httputil.NewClientConn(conn, nil)
	defer client.Close()

	resp, _ := client.Do(req)
	resp = proxy.InterceptResponse(req, resp)

	hdr := w.Header()
	for k, vs := range resp.Header {
		for _, v := range vs {
			hdr.Add(k, v)
		}
	}
	Debug.Printf("Response from target: %s %v", resp.Status, w.Header())

	if resp.Header.Get("Content-Type") == RAW_STREAM {
		w.WriteHeader(resp.StatusCode)
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

		up, rem := client.Hijack()
		defer up.Close()

		end := make(chan bool)

		go func() {
			defer close(end)
			_, err := io.Copy(down, rem)
			_, err = io.Copy(down, up)
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

	} else if resp.TransferEncoding != nil &&
		resp.TransferEncoding[0] == "chunked" {
		// Because we can't go back to request/response after we
		// hijack the connection, we need to close it and make the
		// client open another.
		hdr.Add("Connection", "close")
		w.WriteHeader(resp.StatusCode)
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "Unable to hijack response stream for chunked response", http.StatusInternalServerError)
			return
		}
		down, _, err := hj.Hijack()
		if err != nil {
			http.Error(w, "Unable to hijack response stream for chunked response", http.StatusInternalServerError)
			return
		}
		defer down.Close()

		up, rem := client.Hijack()
		defer up.Close()

		chunkreader := httputil.NewChunkedReader(io.MultiReader(rem, up))
		chunkwriter := httputil.NewChunkedWriter(down)
		io.Copy(chunkwriter, chunkreader)
		chunkwriter.Close()
		resp.Trailer.Write(down)
		// a chunked response ends with a CRLF
		down.Write([]byte{13, 10})
	} else {
		w.WriteHeader(resp.StatusCode)
		_, err = io.Copy(w, resp.Body)
		if err != nil {
			Warning.Print(err)
		}
	}
}

func (proxy *Proxy) RoundTrip(req *http.Request) (*http.Response, error) {
	t := &http.Transport{
		Proxy: nil,
		Dial: func(string, string) (net.Conn, error) {
			return proxy.Dial()
		},
	}
	res, err := t.RoundTrip(req)
	return res, err
}
