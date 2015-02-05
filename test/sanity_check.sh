#! /bin/bash

. ./config.sh

set -e

boldly echo Ping each host from the other
run_on $HOST2 ping -q -c 4 $HOST1
run_on $HOST1 ping -q -c 4 $HOST2

boldly echo Check we can reach docker
docker_on $HOST1 info
docker_on $HOST2 info
