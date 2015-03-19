# Weave Docker Extension design

The current model is to do RPCs at certain points during a container
lifecycle. These are the weave features, lifecycle points, and the
likely RPC signatures that weave features would need to work.

## MVP

The MVP would be an extension that does initialisation and can assign
IP addresses on the weave network to containers when they are started.

### Initialisation

At present weave doesn't need any arguments for initialisation. It may
be passed the IP of other weave peers.

    docker plugin load weaveworks/weave-extension [<peer IP>...]

When started, weave needs to create a bridge device, mess with
iptables, and start a privileged container with the router process.

### IP assignment

At present, to assign an IP to a container when running it, weave
needs to be given an IP address and subnet. The simplest way of
representing this in an API is something like:

    docker run --net=weave://10.2.45.3/24 ...

Weave needs to do its work *after* the container's namespace is
created, and *before* the container's entrypoint is exec'd.

As an RPC, this could look like:

    POST /v1/net/attach
    {"Endpoint": "10.2.45.3/24",
     "ContainerID": "ca88a6e"}
    
    200 OK
    {"IP": "10.2.45.3"}

(I'm assuming the host wants to know what was actuated so it can e.g.,
report it in docker inspect)

This leaves some room for

    docker run --net=weave://net1
    docker run --net=weave://

and other magicalism.

## IP allocation

In progress is an IPAM (IP address management) system for weave
networks. This behaves a bit like DHCP, and would allow invocations
like

    # Allocate an IP on a given subnet
    docker run --net=weave://10.2.67.0/24 ...

This would not require changes to the RPC signature.

## Resolver configuration and DNS

## Considerations for swarm

## Considerations for compose
