package main

import (
	"bytes"
	"encoding/json"
	"flag"
	. "github.com/zettio/weave/common"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
)

const (
	WEAVE = "./weave"
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

// ---- Proxy implementation

func containerInfo(proxy *Proxy, containerId string) (body map[string]interface{}, err error) {
	body = nil
	client := &http.Client{
		Transport: proxy,
	}
	if res, err := client.Get("http://localhost/v1.16/containers/" + containerId + "/json"); err == nil {
		if bs, err := ioutil.ReadAll(res.Body); err == nil {
			err = json.Unmarshal(bs, &body)
		} else {
			Warning.Print("Could not parse response from docker", err)
		}
	} else {
		Warning.Print("Error fetching container info from docker", err)
	}
	return
}

func isCreate(r *http.Request) bool {
	ok, err := regexp.MatchString("^/v[0-9\\.]*/containers/create$", r.URL.Path)
	return err == nil && ok
}

func isStart(r *http.Request) bool {
	ok, err := regexp.MatchString("^/v[0-9\\.]*/containers/[^/]*/start$", r.URL.Path)
	return err == nil && ok
}

func containerFromPath(path string) string {
	if subs := regexp.MustCompile("^/v[0-9\\.]*/containers/([^/]*)/.*").FindStringSubmatch(path); subs != nil {
		return subs[1]
	}
	return ""
}

func callWeave(args ...string) ([]byte, error) {
	Debug.Print("Calling weave", args)
	cmd := exec.Command(WEAVE, args...)
	cmd.Env = []string{"PROCFS=/hostproc", "PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
	out, err := cmd.CombinedOutput()
	return out, err
}

func weaveAddrFromConfig(config map[string]interface{}) string {
	if entries, ok := config["Env"].([]interface{}); ok {
		for _, e := range entries {
			entry := e.(string)
			if strings.Index(entry, "WEAVE_CIDR=") == 0 {
				return entry[11:]
			}
		}
	} else {
		Warning.Print("Unexpected format for config", config)
	}
	return ""
}

func (proxy *Proxy) InterceptRequest(r *http.Request) (*http.Request, error) {
	body := r.Body
	if isCreate(r) {
		var config map[string]interface{}
		bs, _ := ioutil.ReadAll(body)
		body = ioutil.NopCloser(bytes.NewReader(bs))
		if err := json.Unmarshal(bs, &config); err == nil {
			if cidr := weaveAddrFromConfig(config); cidr != "" {
				Info.Printf("Creating container with CIDR %s", cidr)
			}
		} else {
			Warning.Print("Unable to decode CREATE body: ", err)
		}
	}
	req, err := http.NewRequest(r.Method, r.URL.Path, body)
	if err != nil {
		return nil, err
	}
	req.Header = r.Header
	req.URL.RawQuery = r.URL.RawQuery
	return req, nil
}

func (proxy *Proxy) InterceptResponse(req *http.Request, res *http.Response) *http.Response {
	if isStart(req) {
		if containerId := containerFromPath(req.URL.Path); containerId != "" {
			if info, err := containerInfo(proxy, containerId); err == nil {
				Debug.Print(info)
				if cidr := weaveAddrFromConfig(info["Config"].(map[string]interface{})); cidr != "" {
					if out, err := callWeave("--local", "attach", cidr, containerId); err != nil {
						Warning.Print("Calling weave failed:", err, string(out))
					}
				} else {
					Debug.Print("No Weave CIDR, ignoring")
				}
			} else {
				Warning.Print("Unable to fetch container info: ", err)
			}
		} else {
			Error.Fatal("Cound not extract container ID from path")
		}
	}
	return res
}

func (proxy *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Info.Printf("%s %s", r.Method, r.URL)
	req, err := proxy.InterceptRequest(r)
	if err != nil {
		http.Error(w, "Unable to create proxied request", http.StatusInternalServerError)
		Warning.Print(err)
		return
	}
	req.Close = false

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
