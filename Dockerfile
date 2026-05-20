FROM golang:1.26-alpine AS build-env

WORKDIR /app
COPY . .
RUN go build -o /tmp/goblet-server ./goblet-server
RUN go build -o /tmp/packobjectshook ./hooks/packobjects

FROM alpine:3.23
RUN apk add --no-cache git
COPY --from=build-env /tmp/goblet-server /tmp/packobjectshook /app/
COPY example_config.json /app/
WORKDIR /app
RUN ["./goblet-server", "-config", "example_config.json", "-check"]
ENTRYPOINT ["./goblet-server"]
