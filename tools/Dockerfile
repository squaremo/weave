FROM gliderlabs/alpine
MAINTAINER Weaveworks Inc <help@weave.works>
WORKDIR /home/weave
RUN ["apk-install", "ethtool", "conntrack-tools", "curl", "iptables", "iproute2", "docker"]
ADD ./driver /home/weave/
ADD ./proxy  /home/weave/
ENTRYPOINT ["/home/weave/driver"]
