FROM golang:1.24 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/ops-broker ./cmd/ops-broker

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/ops-broker /ops-broker
USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/ops-broker"]
