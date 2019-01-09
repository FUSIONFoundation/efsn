# Build Efsn in a stock Go builder container
FROM golang:1.11.4-alpine as builder

RUN apk add --no-cache make gcc musl-dev linux-headers

ADD . /efsn
RUN cd /efsn && make efsn

# Pull Geth into a second stage deploy alpine container
FROM alpine:latest

RUN apk add --no-cache ca-certificates
# RUN apk add --no-cache jq
COPY --from=builder /efsn/build/bin/efsn /usr/local/bin/

EXPOSE 40401 40401/udp 40402 40402/udp 40404 40404/udp 9001 9001/udp 8001 8001/udp 16714 16714/udp

COPY ./docker-entrypoint.sh /usr/local/bin

RUN chmod a+x /usr/local/bin/docker-entrypoint.sh \
  && ln -s /usr/local/bin/docker-entrypoint.sh / # Needed for backwards compatability

ENTRYPOINT ["docker-entrypoint.sh"]