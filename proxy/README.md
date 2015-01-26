# Weave Docker proxy

The Weave Docker proxy provides a convenient way to integrate weave
with Docker; in particular, it means you can start containers using
the docker client (or other docker API-using tool) and have them
attach to a weave network. It's also a simple way to use weave
remotely.

## Example

To start things off, it's necessary to launch weave:

```bash
$ weave launch
```

> This starts the proxy, which in turns asks docker to run the router
> (if it's not running). The proxy container has everything it needs
> for actuating weave operations.

This makes the proxy available on `:12375`. It's also possible to
launch weave with docker, but it's more involved:

```bash
$ docker run -d -v /var/run/docker.sock:/var/run/docker.sock -p 12375:12375 zettio/weavetools launch
```

You can now talk to the proxy as though it were the Docker daemon
itself. Containers given an environment entry of `WEAVE_CIDR` will be
given an interface on the weave network.

```bash
$ export DOCKER_HOST=tcp://localhost:12375
$ docker run -e WEAVE_CIDR=10.1.1.5/24 -ti ubuntu
```

The weave script operates normally, including using `weave run`.

```bash
$ weave run 10.3.5.6/24 -ti --name=foo ubuntu
$ weave attach 10.5.2.12/24 foo
```

> For this to be the case, the weave script passes its arguments along
> to a container on the host, which calls the "main" script with the
> arguments.

```bash
$ docker run --rm zettio/weavetools attach 10.5.2.12/24 foo
```
