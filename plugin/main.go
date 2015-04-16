package main

import (
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/mux"
	. "github.com/weaveworks/weave/common"
	"io"
	"math/big"
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
	State  map[string]string
}

type endpointInfo struct {
	ID      string
	Network string
	Labels  map[string]string
}

type netInterface struct {
	Gateway     string `json:"gateway"`
	IPAddress   string `json:"ip"`
	IPPrefixLen uint   `json:"ip_prefix_len"`
	MacAddress  string `json:"mac"`
	Bridge      string `json:"bridge"`
}

var (
	network *networkInfo
	subnet  *net.IPNet
	peers   []string
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

	network = &info

	sub, exists := info.Labels["subnet"]
	if !exists {
		sub = "10.2.0.0/16"
	}
	_, subnet, err = net.ParseCIDR(sub)
	if err != nil {
		http.Error(w, "Invalid subnet CIDR", http.StatusBadRequest)
		return
	}
	if err = doWeaveCmd(append([]string{"launch"}, peers...)); err != nil {
		http.Error(w, "Problem launching Weave: "+err.Error(), http.StatusInternalServerError)
		return
	}
	Info.Printf("Create network")
	w.Write([]byte{})
}

func destroyNetwork(w http.ResponseWriter, r *http.Request) {
	routeVars := mux.Vars(r)
	netID, _ := routeVars["networkID"]
	if err := doWeaveCmd([]string{"stop"}); err != nil {
		http.Error(w, "Unable to stop weave: "+err.Error(), http.StatusInternalServerError)
		return
	}
	Info.Printf("Destroy network %s", netID)
	w.Write([]byte{})
}

func plugEndpoint(w http.ResponseWriter, r *http.Request) {
	var info endpointInfo
	err := json.NewDecoder(r.Body).Decode(&info)
	if err != nil {
		http.Error(w, "Cannot parse JSON", http.StatusBadRequest)
		return
	}
	routeVars := mux.Vars(r)
	netID, _ := routeVars["networkID"]
	if network == nil || netID != network.ID {
		notFound(w, r)
		return
	}
	ip := makeIP(subnet)
	mac := makeMac(ip)
	prefix, _ := subnet.Mask.Size()
	resp := netInterface{
		Gateway:     "",
		IPAddress:   ip.String(),
		IPPrefixLen: uint(prefix),
		MacAddress:  mac,
		Bridge:      "weave",
	}
	Debug.Printf("Plug: %+v", &resp)
	if err = json.NewEncoder(w).Encode(&resp); err != nil {
		http.Error(w, "Could not JSON encode response", http.StatusInternalServerError)
	}
	Info.Printf("Plug endpoint %s %+v", info.ID, resp)
}

func unplugEndpoint(w http.ResponseWriter, r *http.Request) {
	routeVars := mux.Vars(r)
	endID := routeVars["endpointID"]
	w.Write([]byte{})
	Info.Printf("Unplug endpoint %s", endID)
}

// ===

func doWeaveCmd(args []string) error {
	cmd := exec.Command("./weave", args...)
	cmd.Env = []string{"PATH=/usr/bin:/usr/local/bin"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		Warning.Print(string(out))
	}
	return err
}

func makeIP(sub *net.IPNet) net.IP {
	var base uint = uint(sub.IP[3]) + uint(sub.IP[2])<<8 + uint(sub.IP[1])<<16 + uint(sub.IP[0])<<24
	ones, bits := sub.Mask.Size()
	add, _ := rand.Int(rand.Reader, big.NewInt((1<<uint(bits-ones))-2))
	asInt := uint64(base) + add.Uint64()
	return net.IPv4(byte(asInt>>24), byte(asInt>>16), byte(asInt>>8), byte(asInt))
}

func makeMac(ip net.IP) string {
	hw := make(net.HardwareAddr, 6)
	hw[0] = 0x7a
	hw[1] = 0x42
	copy(hw[2:], ip.To4())
	return hw.String()
}
