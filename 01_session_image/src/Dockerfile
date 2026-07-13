FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /session ./cmd/session

# distroless static carries the CA certificates the TLS connections to
# OpenAI and the Google APIs need; scratch does not.
FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /session /session
ENTRYPOINT ["/session"]
