FROM alpine:3.7

RUN \
  apk add --update go git make gcc musl-dev linux-headers ca-certificates && \
  git clone --depth 1 https://github.com/FusionFoundation/efsn && \
  (cd go-ethereum && make geth) && \
  cp efsn/build/bin/geth /geth && \
  apk del go git make gcc musl-dev linux-headers && \
  rm -rf /go-ethereum && rm -rf /var/cache/apk/*

EXPOSE 8545
EXPOSE 40404

ENTRYPOINT ["/geth"]
