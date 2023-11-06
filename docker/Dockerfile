ARG NODE_VERSION=21        # https://hub.docker.com/_/node/tags?page=1&name=19
ARG GOSU_VERSION=1.16      # https://github.com/tianon/gosu/releases

FROM alpine/curl as build_gosu
ARG GOSU_VERSION

RUN curl -fsLo /usr/local/bin/gosu https://github.com/tianon/gosu/releases/download/${GOSU_VERSION}/gosu-$([ "$(uname -m)" = "aarch64" ] && echo "arm64" || echo "amd64")
RUN chmod +x /usr/local/bin/gosu

FROM node:${NODE_VERSION}

WORKDIR /app/dashboard

COPY entrypoint_dev.sh /usr/local/bin/entrypoint_dev.sh
COPY --from=build_gosu /usr/local/bin/gosu /usr/local/bin/gosu

ENTRYPOINT ["/usr/local/bin/entrypoint_dev.sh"]
CMD true
