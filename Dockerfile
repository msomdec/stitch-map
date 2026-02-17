FROM golang:1.25-alpine AS build

RUN go install github.com/a-h/templ/cmd/templ@latest

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN templ generate
RUN CGO_ENABLED=0 go build -o /bin/server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /bin/server /bin/server

RUN mkdir -p /data
EXPOSE 8080

ENTRYPOINT ["/bin/server"]
CMD ["--addr", ":8080", "--db", "/data/stitchmap.db"]
