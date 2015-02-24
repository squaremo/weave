# NB only to be sourced

set -e

# these ought to match what is in Vagrantfile
N_MACHINES=${N_MACHINES:-2}
IP_PREFIX=${IP_PREFIX:-192.168.48}
IP_SUFFIX_BASE=${IP_SUFFIX_BASE:-10}

HOSTS=
for i in $(seq 1 $N_MACHINES); do
    IP="${IP_PREFIX}.$((${IP_SUFFIX_BASE}+$i))"
    HOSTS="$HOSTS $IP"
done

# these are used by the tests
HOST1="${IP_PREFIX}.$((${IP_SUFFIX_BASE}+1))"
HOST2="${IP_PREFIX}.$((${IP_SUFFIX_BASE}+2))"

. ./assert.sh

SSH=${SSH:-ssh -l vagrant -i ./insecure_private_key -o UserKnownHostsFile=./.ssh_known_hosts -o CheckHostIP=no -o StrictHostKeyChecking=no}

remote() {
    rem=$1
    shift 1
    $@ > >(while read line; do echo -e "\e[0;34m$rem>\e[0m $line"; done)
}

whitely() {
    echo -e '\e[1;37m'`$@`'\e[0m'
}

greyly () {
    echo -e '\e[0;37m'`$@`'\e[0m'
}

run_on() {
    host=$1
    shift 1
    greyly echo "Running on $host: $@"
    remote $host $SSH $host $@
}

docker_on() {
    host=$1
    shift 1
    greyly echo "Docker on $host: $@"
    docker -H tcp://$host:2375 $@
}

weave_on() {
    host=$1
    shift 1
    greyly echo "Weave on $host: $@"
    DOCKER_HOST=tcp://$host:2375 $WEAVE $@
}

start_suite() {
    whitely echo $@
}

end_suite() {
    whitely assert_end
}

WEAVE=../weave
DOCKER_NS=./bin/docker-ns
