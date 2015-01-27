FROM gliderlabs/alpine
MAINTAINER Weaveworks Inc <help@weave.works>
WORKDIR /home/weave
RUN ["apk-install", "ethtool", "conntrack-tools", "curl", "iptables", "iproute2"]
ADD ./driver /home/weave/
ENTRYPOINT ["/home/weave/driver"]
