FROM node:17-stretch

RUN curl -Lo /usr/local/bin/gosu https://github.com/tianon/gosu/releases/download/1.14/gosu-amd64 && chmod +x /usr/local/bin/gosu

COPY entrypoint_dev.sh /usr/local/bin/entrypoint_dev.sh

ENTRYPOINT ["/usr/local/bin/entrypoint_dev.sh"]

CMD true

WORKDIR /app/dashboard
