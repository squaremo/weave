package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/mux"
	. "github.com/weaveworks/weave/common"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
)

var version = "(unreleased version)"

type handshakeResp struct {
	InterestedIn []string
	Name         string
	Author       string
	Org          string
	Website      string
}

type networkInfo struct {
	Name   string
	ID     string
	Driver string
	Labels map[string]string
}

var (
	subnet *net.IPNet
	peers  []string
)

func main() {
	var (
		justVersion bool
		address     string
		debug       bool
	)

	flag.BoolVar(&justVersion, "version", false, "print version and exit")
	flag.BoolVar(&debug, "debug", false, "output debugging info to stderr")
	flag.StringVar(&address, "socket", "/var/run/docker-plugin/plugin.sock", "socket on which to listen")

	flag.Parse()

	if justVersion {
		fmt.Printf("weave plugin %s\n", version)
		os.Exit(0)
	}

	InitDefaultLogging(debug)

	peers = flag.Args()

	router := mux.NewRouter()
	router.NotFoundHandler = http.HandlerFunc(notFound)
	router.Methods("GET").Path("/status").HandlerFunc(status)

	router.Methods("POST").Path("/v1/handshake").HandlerFunc(handshake)

	router.Methods("POST").Path("/v1/net/").HandlerFunc(createNetwork)
	router.Methods("DELETE").Path("/v1/net/{networkID}").HandlerFunc(destroyNetwork)

	router.Methods("POST").Path("/v1/net/{networkID}/").HandlerFunc(plugEndpoint)
	router.Methods("DELETE").Path("/v1/net/{networkID}/{endpointID}").HandlerFunc(unplugEndpoint)

	var listener net.Listener

	listener, err := net.Listen("unix", address)
	if err != nil {
		Error.Fatalf("[plugin] Unable to listen on %s: %s", address, err)
	}

	if err := http.Serve(listener, router); err != nil {
		Error.Fatalf("[plugin] Internal error: %s", err)
	}
}

func notFound(w http.ResponseWriter, r *http.Request) {
	Warning.Printf("[plugin] Not found: %s", r.URL.Path)
	http.NotFound(w, r)
}

func handshake(w http.ResponseWriter, r *http.Request) {
	err := json.NewEncoder(w).Encode(&handshakeResp{
		[]string{"net"},
		"weave",
		"help@weave.works",
		"WeaveWorks",
		"http://weave.works/",
	})
	if err != nil {
		Error.Fatal("handshake encode:", err)
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
	Info.Printf("Handshake completed")
}

func status(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, fmt.Sprintln("weave plugin", version))
}

func createNetwork(w http.ResponseWriter, r *http.Request) {
	var info networkInfo
	err := json.NewDecoder(r.Body).Decode(&info)
	if err != nil {
		http.Error(w, "Cannot parse JSON", http.StatusBadRequest)
		return
	}
	sub, exists := info.Labels["subnet"]
	if !exists {
		sub = "10.2.0.0/16"
	}
	_, subnet, err = net.ParseCIDR(sub)
	if err != nil {
		http.Error(w, "Invalid subnet CIDR", http.StatusBadRequest)
		return
	}
	if err = launchWeave(); err != nil {
		http.Error(w, "Problem launching Weave: "+err.Error(), http.StatusInternalServerError)
		return
	}
	Info.Printf("Create network")
	w.Write([]byte{})
}

func destroyNetwork(w http.ResponseWriter, r *http.Request) {
	Info.Printf("Destroy network")
	http.Error(w, "Unimplemented", http.StatusNotImplemented)
}

func plugEndpoint(w http.ResponseWriter, r *http.Request) {
	Info.Printf("Plug endpoint")
	http.Error(w, "Unimplemented", http.StatusNotImplemented)
}

func unplugEndpoint(w http.ResponseWriter, r *http.Request) {
	Info.Printf("Unplug endpoint")
	http.Error(w, "Unimplemented", http.StatusNotImplemented)
}

// ===

func launchWeave() error {
	args := []string{"launch"}
	args = append(args, peers...)
	cmd := exec.Command("./weave", args...)
	cmd.Env = []string{"PATH=/usr/bin:/usr/local/bin"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		Warning.Print(string(out))
	}
	return err
}
