##################################
# STEP 1 build executable binary #
##################################
FROM golang:alpine AS builder

# Copy sources.
COPY . $GOPATH/src/github.com/scotow/bagaar

# Move to command directory.
WORKDIR $GOPATH/src/github.com/scotow/bagaar

# Build the binary.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /go/bin/bagaar

##############################
# STEP 2 build a small image #
##############################
FROM scratch

# Copy our static executable and static files.
COPY --from=builder /go/bin/bagaar /bagaar

# Copy SSL certificates for HTTPS connections.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Run the hello binary.
ENTRYPOINT ["/bagaar"]