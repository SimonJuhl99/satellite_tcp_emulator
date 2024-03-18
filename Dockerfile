#ADD file:37a76ec18f9887751cd8473744917d08b7431fc4085097bb6a09d81b41775473 in /  # alpine 3.19 amd64
FROM alpine:3.19
CMD ["/bin/sh"]
RUN apk add iproute2 iperf3
RUN apk add tcpdump bash bash-completion iptraf-ng
COPY tcp_metrics /tcp_metrics
CMD ["/bin/sleep", "infinity"]