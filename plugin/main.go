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
	Implements []string
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

	router.Methods("POST").Path("/Plugin.Activate").HandlerFunc(handshake)

	router.Methods("POST").Path("/NetworkDriver.CreateNetwork").HandlerFunc(createNetwork)
	router.Methods("POST").Path("/NetworkDriver.DeleteNetwork").HandlerFunc(deleteNetwork)

	router.Methods("POST").Path("/NetworkDriver.CreateEndpoint").HandlerFunc(createEndpoint)
	router.Methods("POST").Path("/NetworkDriver.DeleteEndpoint").HandlerFunc(deleteEndpoint)

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
		[]string{"NetworkDriver"},
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

type networkCreate struct {
	Id      string
	Options map[string]interface{}
}

func createNetwork(w http.ResponseWriter, r *http.Request) {
	sub := "10.2.0.0/16"
	var err error
	_, subnet, err = net.ParseCIDR(sub)
	if err != nil {
		http.Error(w, "Invalid subnet CIDR", http.StatusBadRequest)
		return
	}

	var create networkCreate
	err = json.NewDecoder(r.Body).Decode(&create)
	if err != nil {
		http.Error(w, "Unable to decode JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	Debug.Printf("Create network request %+v", &create)

	network = create.Id

	weaveArgs := []string{"launch", "-iprange", subnet.String()}
	if _, err = doWeaveCmd(weaveArgs); err != nil {
		http.Error(w, "Problem launching Weave: "+err.Error(), http.StatusInternalServerError)
		return
	}
	Info.Printf("Create network %s", network)
	w.Write([]byte{})
}

type networkDelete struct {
	NetworkId string
}

func deleteNetwork(w http.ResponseWriter, r *http.Request) {
	var delete networkDelete
	if err := json.NewDecoder(r.Body).Decode(&delete); err != nil {
		http.Error(w, "Unable to decode JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	if delete.NetworkId != network {
		notFound(w, r)
		return
	}
	if _, err := doWeaveCmd([]string{"stop"}); err != nil {
		http.Error(w, "Unable to stop weave: "+err.Error(), http.StatusInternalServerError)
		return
	}
	network = ""
	Info.Printf("Destroy network %s", delete.NetworkId)
	w.Write([]byte{})
}

type endpointCreate struct {
	NetworkId  string
	EndpointId string
	Options    map[string]interface{}
}

func createEndpoint(w http.ResponseWriter, r *http.Request) {
	var create endpointCreate
	if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
		http.Error(w, "Unable to decode JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	Debug.Printf("Create endpoint request %+v", &create)
	netID := create.NetworkId
	endID := create.EndpointId

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
	}
	resp := &sbInfo{
		Interfaces:  []*iface{respIface},
		Gateway:     nil,
		GatewayIPv6: nil,
	}

	Debug.Printf("Plug: %+v", &resp)
	if err = json.NewEncoder(w).Encode(&resp); err != nil {
		http.Error(w, "Could not JSON encode response", http.StatusInternalServerError)
		return
	}
	Info.Printf("Create endpoint %s %+v", endID, resp)
}

type endpointDelete struct {
	NetworkId  string
	EndpointId string
}

func deleteEndpoint(w http.ResponseWriter, r *http.Request) {
	var delete endpointDelete
	if err := json.NewDecoder(r.Body).Decode(&delete); err != nil {
		http.Error(w, "Could not decode JSON encode payload", http.StatusBadRequest)
		return
	}
	// TODO
	w.Write([]byte{})
	Info.Printf("Delete endpoint %s", delete.EndpointId)
}

type join struct {
	NetworkId  string
	EndpointId string
	SandboxKey string
	Options    map[string]interface{}
}

type joinResponse struct {
	HostPath string
}

func joinEndpoint(w http.ResponseWriter, r *http.Request) {
	var j join
	if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
		http.Error(w, "Could not decode JSON encode payload", http.StatusBadRequest)
		return
	}
	// TODO
	if err := json.NewEncoder(w).Encode(&joinResponse{""}); err != nil {
		http.Error(w, "Could not JSON encode response", http.StatusInternalServerError)
		return
	}
	Info.Printf("Join endpoint %s:%s to %s", j.NetworkId, j.EndpointId, j.SandboxKey)
}

type leave struct {
	NetworkId  string
	EndpointId string
	Options    map[string]interface{}
}

func leaveEndpoint(w http.ResponseWriter, r *http.Request) {
	var l leave
	if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
		http.Error(w, "Could not decode JSON encode payload", http.StatusBadRequest)
		return
	}
	// TODO
	w.Write([]byte{})
	Info.Printf("Delete endpoint %s:%s", l.NetworkId, l.EndpointId)
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
