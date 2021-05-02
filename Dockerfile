# Build
FROM golang:1.16-buster AS build

WORKDIR /app

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY *.go .
RUN go build -o /app/temposaurus

# Deploy
FROM gcr.io/distroless/base-debian10

WORKDIR /app

COPY --from=build /app/temposaurus .

USER nonroot:nonroot

ENTRYPOINT ["/app/temposaurus"]
