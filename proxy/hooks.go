package proxy

import (
	"bytes"
	"encoding/json"
	. "github.com/zettio/weave/common"
	"io/ioutil"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
)

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
	args = append([]string{"--local"}, args...)
	Debug.Print("Calling weave", args)
	cmd := exec.Command("./weave", args...)
	cmd.Env = []string{"PROCFS=/hostproc", "PATH=/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
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
					if out, err := callWeave("attach", cidr, containerId); err != nil {
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
