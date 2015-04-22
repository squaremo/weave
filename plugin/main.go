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

type iface struct {
	SrcName    string
	DstName    string
	Address    string
	MACAddress string
}

type sbInfo struct {
	Interfaces  []*iface
	Gateway     net.IP
	GatewayIPv6 net.IP
}

var (
	network string
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

	router.Methods("PUT").Path("/v1/net/{networkID}").HandlerFunc(createNetwork)
	router.Methods("DELETE").Path("/v1/net/{networkID}").HandlerFunc(destroyNetwork)

	router.Methods("PUT").Path("/v1/net/{networkID}/{endpointID}").HandlerFunc(plugEndpoint)
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
	Warning.Printf("[plugin] Not found: %+v", r)
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
	routeVars := mux.Vars(r)
	netID, _ := routeVars["networkID"]
	network = netID

	sub := "10.2.0.0/16"
	var err error
	_, subnet, err = net.ParseCIDR(sub)
	if err != nil {
		http.Error(w, "Invalid subnet CIDR", http.StatusBadRequest)
		return
	}

	weaveArgs := []string{"launch", "-alloc", subnet.String()}
	if _, err = doWeaveCmd(weaveArgs); err != nil {
		http.Error(w, "Problem launching Weave: "+err.Error(), http.StatusInternalServerError)
		return
	}
	Info.Printf("Create network")
	w.Write([]byte{})
}

func destroyNetwork(w http.ResponseWriter, r *http.Request) {
	routeVars := mux.Vars(r)
	netID, _ := routeVars["networkID"]
	if _, err := doWeaveCmd([]string{"stop"}); err != nil {
		http.Error(w, "Unable to stop weave: "+err.Error(), http.StatusInternalServerError)
		return
	}
	Info.Printf("Destroy network %s", netID)
	w.Write([]byte{})
}

func plugEndpoint(w http.ResponseWriter, r *http.Request) {
	routeVars := mux.Vars(r)
	netID, _ := routeVars["networkID"]
	endID, _ := routeVars["endpointID"]
	if netID != network {
		notFound(w, r)
		return
	}
	ip, err := getIP(endID)
	if err != nil {
		Warning.Printf("Error allocating IP:", err)
		http.Error(w, "Unable to allocate IP", http.StatusInternalServerError)
		return
	}
	prefix, _ := subnet.Mask.Size()

	// use the endpoint ID bytes to make veth names, and cross fingers
	localName := "vethwel" + endID[:5]
	guestName := "vethweg" + endID[:5]
	mac := makeMac(ip)
	// create and attach local name to the bridge
	ipout, err := doIpCmd([]string{"link", "add", "name", localName, "type", "veth", "peer", "name", guestName})
	if err != nil {
		Warning.Print(ipout)
		http.Error(w, "Could not configure net device", http.StatusInternalServerError)
		return
	}
	ipout, err = doIpCmd([]string{"link", "set", localName, "master", "weave"})
	if err != nil {
		Warning.Print(ipout)
		http.Error(w, "Could not configure net device", http.StatusInternalServerError)
		return
	}
	ipout, err = doIpCmd([]string{"link", "set", localName, "up"})
	if err != nil {
		Warning.Print(ipout)
		http.Error(w, "Could not configure net device", http.StatusInternalServerError)
		return
	}

	respIface := &iface{
		SrcName: guestName,
		DstName: "ethwe",
		Address: (&net.IPNet{
			ip,
			net.CIDRMask(prefix, 32),
		}).String(),
		MACAddress: mac,
	}
	resp := &sbInfo{
		Interfaces:  []*iface{respIface},
		Gateway:     nil,
		GatewayIPv6: nil,
	}

	Debug.Printf("Plug: %+v", &resp)
	if err = json.NewEncoder(w).Encode(&resp); err != nil {
		http.Error(w, "Could not JSON encode response", http.StatusInternalServerError)
	}
	Info.Printf("Plug endpoint %s %+v", endID, resp)
}

func unplugEndpoint(w http.ResponseWriter, r *http.Request) {
	routeVars := mux.Vars(r)
	endID := routeVars["endpointID"]
	w.Write([]byte{})
	Info.Printf("Unplug endpoint %s", endID)
}

// ===

func doWeaveCmd(args []string) (string, error) {
	cmd := exec.Command("./weave", args...)
	cmd.Env = []string{"PATH=/usr/bin:/usr/local/bin"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		Warning.Print(string(out))
	}
	return string(out), err
}

func doIpCmd(args []string) (string, error) {
	cmd := exec.Command("ip", args...)
	cmd.Env = []string{"PATH=/usr/bin:/usr/local/bin"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		Warning.Print(string(out))
	}
	return string(out), err
}

// assumed to be in the subnet
func getIP(ID string) (net.IP, error) {
	res, err := doWeaveCmd([]string{"alloc", ID})
	if err != nil {
		return nil, err
	}
	ip, _, err := net.ParseCIDR(res)
	return ip, err
}

func makeMac(ip net.IP) string {
	hw := make(net.HardwareAddr, 6)
	hw[0] = 0x7a
	hw[1] = 0x42
	copy(hw[2:], ip.To4())
	return hw.String()
}
