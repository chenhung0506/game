FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/game .

FROM alpine:3.20
RUN adduser -D -u 10001 game
# The server reads static/ and config.json relative to its working directory.
WORKDIR /app
COPY --from=build /out/game .
COPY static ./static
COPY config.json .
USER game
ENV PORT=8080
EXPOSE 8080
CMD ["./game"]
