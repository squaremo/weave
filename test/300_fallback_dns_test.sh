#! /bin/bash

. ./config.sh

start_suite "Resolve a non-weave address"

run_on $HOST1 sudo $WEAVE stop-dns || true
run_on $HOST1 sudo $WEAVE launch-dns 10.2.254.1/24 -debug
docker_on $HOST1 rm -f c1 || true

run_on $HOST1 sudo $WEAVE run --with-dns 10.2.1.5/24 --name=c1 -t aanand/docker-dnsutils /bin/sh

ok=$(docker -H tcp://$HOST1:2375 exec -i c1 dig +short -t MX weave.works)
assert "test -n \"$ok\" && echo pass" "pass"

ok=$(docker -H tcp://$HOST1:2375 exec -i c1 dig +short -x 8.8.8.8)
assert "test -n \"$ok\" && echo pass" "pass"

end_suite
