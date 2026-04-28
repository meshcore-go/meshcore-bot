FROM alpine:3.23

ARG TARGETPLATFORM

RUN apk add --no-cache ca-certificates tzdata

COPY bin/ /tmp/bin/

RUN case "${TARGETPLATFORM}" in \
      "linux/386")      BIN="meshcore-bot-linux-386/meshcore-bot-linux-386" ;; \
      "linux/amd64")    BIN="meshcore-bot-linux-amd64/meshcore-bot-linux-amd64" ;; \
      "linux/arm/v6")   BIN="meshcore-bot-linux-armv6/meshcore-bot-linux-armv6" ;; \
      "linux/arm/v7")   BIN="meshcore-bot-linux-armv7/meshcore-bot-linux-armv7" ;; \
      "linux/arm64"*)   BIN="meshcore-bot-linux-arm64/meshcore-bot-linux-arm64" ;; \
      "linux/ppc64le")  BIN="meshcore-bot-linux-ppc64le/meshcore-bot-linux-ppc64le" ;; \
      "linux/riscv64")  BIN="meshcore-bot-linux-riscv64/meshcore-bot-linux-riscv64" ;; \
      "linux/s390x")    BIN="meshcore-bot-linux-s390x/meshcore-bot-linux-s390x" ;; \
      *) echo "unsupported platform: ${TARGETPLATFORM}" && exit 1 ;; \
    esac && \
    cp "/tmp/bin/${BIN}" /usr/local/bin/meshcore-bot && \
    chmod +x /usr/local/bin/meshcore-bot && \
    rm -rf /tmp/bin

ENTRYPOINT ["meshcore-bot"]
