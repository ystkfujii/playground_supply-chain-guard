FROM golang:1.26.4 AS build
WORKDIR /wd
COPY go.mod go.sum ./
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /wd/provider .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /wd/provider /provider
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/provider"]
