#!/bin/bash

sudo rm -f /usr/share/docker/plugins/weave.sock

docker run --privileged --net=host -d -v /var/run/docker.sock:/var/run/docker.sock -v /usr/share/docker/plugins:/usr/share/docker/plugins weaveworks/plugin --socket=/usr/share/docker/plugins/weave.sock "$@"
