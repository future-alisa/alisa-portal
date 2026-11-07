FROM golang:1.24-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /portal ./cmd/server
FROM alpine:3.22
RUN addgroup -S portal && adduser -S portal -G portal && mkdir /data && chown portal:portal /data
COPY --from=build /portal /usr/local/bin/portal
USER portal
ENV LISTEN_ADDR=0.0.0.0:8080 DATA_FILE=/data/portal.json
EXPOSE 8080
ENTRYPOINT ["portal"]
