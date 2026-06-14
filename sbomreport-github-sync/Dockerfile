FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/sbomreport-github-sync .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/sbomreport-github-sync /usr/local/bin/sbomreport-github-sync
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/sbomreport-github-sync"]
