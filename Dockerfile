FROM golang:1.13.7-alpine3.11 AS build

# Copy source
WORKDIR /app/session-manager
COPY . .

# Download dependencies application
RUN go mod download

# Build application.
WORKDIR /app/session-manager/cmd
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build

FROM alpine:3.11 AS run

WORKDIR /etc/session-manager/migrations
COPY ./resources/db/mysql/ .

WORKDIR /opt/app
RUN ls /etc/session-manager/migrations
COPY --from=build /app/session-manager/cmd/cmd session-manager
ENV GIN_MODE release
CMD ["./session-manager"]