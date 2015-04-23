#!/bin/bash

docker run --privileged --net=host --plugin -d -v /var/run/docker.sock:/var/run/docker.sock zettio/plugin --socket=/var/run/docker-plugin/p.s "$@"
