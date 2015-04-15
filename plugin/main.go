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
)

var version = "(unreleased version)"

type handshakeResp struct {
	InterestedIn []string
	Name         string
	Author       string
	Org          string
	Website      string
}

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
	Info.Printf("Create network")
	http.Error(w, "Unimplemented", http.StatusNotImplemented)
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
