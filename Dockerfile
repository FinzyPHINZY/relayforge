FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGET=api
RUN CGO_ENABLED=0 go build -o /app ./cmd/${TARGET}

FROM alpine:3.22
WORKDIR /app
COPY --from=build /app /app/server
COPY db/migrations /app/db/migrations
ENTRYPOINT ["/app/server"]
