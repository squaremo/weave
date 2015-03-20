# Weave Docker Extension design

The current model is to do RPCs at certain points during a container
lifecycle. These are the weave features, lifecycle points, and the
likely RPC signatures that weave features would need to work.

## Principles

In general we don't like remembering state: to date the only state the
router needs is recoverable from routing tables and other network
configuration, so the router doesn't need to write anything to disk
and can be restarted without interrupting network operation.

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

### Extension point for IP allocation

It's also possible to make IP allocation, or indeed network
configuration, an extension point of its own.

One scheme is to have the network driver invoke the network
configuration via some means given it by the daemon. This gives the
driver the option to ignore the network configuration if it needs to
do its own (some combinations of driver and configuration may not make
sense). However, this may be difficult to arrange with the RPC
mechanism.

Another scheme is for the docker daemon to call the configuration
extension before 

## Resolver configuration and DNS

WeaveDNS provides a resolver on a weave network (or otherwise). In the
default deployment, the weave command will add entries to WeaveDNS for
any container given a hostname; e.g., `weave run
... --hostname=foo.weave.local`, and will tell containers to use
WeaveDNS as the DNS server if the container is run with `weave run
--with-dns ...`. These represent an "opt-in" scheme, but in an
extension we would probably want to have an "opt-out" scheme; that is,
containers on a Weave network are told to use WeaveDNS unless there's
some option supplied.

To enrol a container with WeaveDNS, we need to be told about the
container after it has been given an IP address. It is less sensitive
to ordering with respect to the entrypoint process.

To tell a container to use the WeaveDNS as a nameserver, we need to
provide the `--dns` and `--dnssearch` options to `docker run`, or
otherwise be able to change the contents of `/etc/resolv.conf`.

Potentially this could be done as an API extension rather than a
driver extension.

### WeaveDNS initialisation

Each WeaveDNS peer needs an IP address on the weave network;
potentially we could reserve a special subnet for the peers, but they
still need to be allocated an address on it.

## Considerations for swarm

The docker swarm ideal is that individual hosts can be driven by the
same means as the swarm master -- that is, with the docker remote
API. In the case of supplying explicit information (`docker run
--net=weave://10.2.56.7/24`) this can be given directly to the host
and thereby to the extension.

This may be adequate for some tasks, but it seems likely that users
will also want to control networking of clusters using names rather
than explicit addresses (i.e., more like the model given in
https://github.com/docker/docker/issues/9983). This introduces a
dilemma: should swarm take responsibility for maintaining the cluster
state (in which case it needs to know the network model!), or should
extensions be required to know how to operate in a (swarm) cluster?

My inclination is to make swarm responsible, and to make sure the
docker remote API is up to the task of ferrying the necessary
information back from the network driver.

## Considerations for compose

Docker compose (fig) configurations describe inter-container
communication in terms of links. In that scheme, containers in the
configuration are given names, and containers that need to talk to
another are supplied a link mentioning a container name.

This has a natural interpretation in terms of DNS entries: each name
is a DNS hostname, and each link is simply the (ambient) ability to
resolve a name.

However, docker compose instances applications by constructing fresh
names based on those given in the configuration; e.g., a container
named `redis` in the configuration is given the *actual* name
`redis-1`, and any container with a link to `redis` is actually linked
with the mapping `redi:redis-1`. Obviously this upsets the
interpretation in terms of DNS entries.

A workaround that works now is to use hostnames in the compose
configuration, since this is independent of the links, and is what
weaveDNS would look at anyway. We may want to revisit this later
though, depending on docker compose's trajectory.

## docker network tool

Some of the proposals for docker networking (including the MVP)
introduce a separate docker network tool, which is used to create and
otherwise manipulate networks symbolically (i.e., by name rather than
specification).

The simplest realisation of these networks is as a subnet of some
large address space. These can be allocated as needed; however, it is
still necessary to keep a map of name to subnet somewhere.
