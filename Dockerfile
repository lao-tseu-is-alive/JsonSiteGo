# Start from the latest golang base image
FROM golang:1.25-alpine3.22 AS builder

# Define build arguments for version and build timestamp
ARG APP_REVISION
ARG BUILD
ARG APP_REPOSITORY=https://github.com/lao-tseu-is-alive/JsonSiteGo

# Add Maintainer Info
LABEL maintainer="cgil"
LABEL org.opencontainers.image.title="JsonSiteGo"
LABEL org.opencontainers.image.description="Using the Go programming language to develop a backend API and a frontend interface for creating, managing, and importing tree location data in Lausanne, Switzerland"
LABEL org.opencontainers.image.url="https://ghcr.io/lao-tseu-is-alive/JsonSiteGo:latest"
LABEL org.opencontainers.image.authors="cgil"
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.version="1.0.0"
# Set image version label dynamically
LABEL org.opencontainers.image.source="${APP_REPOSITORY}"

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY "cmd/jsonSiteGoServer" ./jsonSiteGoServer
COPY pkg ./pkg

# Clean the APP_REPOSITORY for ldflags
RUN APP_REPOSITORY_CLEAN=$(echo $APP_REPOSITORY | sed 's|https://||') && \
    CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-w -s -X ${APP_REPOSITORY_CLEAN}/pkg/version.REVISION=${APP_REVISION} -X ${APP_REPOSITORY_CLEAN}/pkg/version.BuildStamp=${BUILD}" -o jsonSiteGoServer ./jsonSiteGoServer


######## Start a new stage  #######
FROM scratch
LABEL author="cgil"
LABEL org.opencontainers.image.authors="cgil"
LABEL description="Using the Go programming language to develop a backend API and a frontend interface for creating, managing, and importing tree location data in Lausanne, Switzerland"
LABEL org.opencontainers.image.description="Using the Go programming language to develop a backend API and a frontend interface for creating, managing, and importing tree location data in Lausanne, Switzerland"
LABEL org.opencontainers.image.url="ghcr.io/lao-tseu-is-alive/JsonSiteGo:latest"
LABEL org.opencontainers.image.source="https://github.com/lao-tseu-is-alive/JsonSiteGo"
# Pass build arguments to the final stage for labeling
ARG APP_REVISION
ARG BUILD
LABEL org.opencontainers.image.version="${APP_REVISION}"
LABEL org.opencontainers.image.revision="${APP_REVISION}"
LABEL org.opencontainers.image.created="${BUILD}"

USER 1221:1221
WORKDIR /goapp

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/jsonSiteGoServer .

ENV PORT="${PORT}"
EXPOSE ${PORT}

# how to check if container is ok https://docs.docker.com/engine/reference/builder/#healthcheck
HEALTHCHECK --start-period=5s --interval=30s --timeout=3s \
    CMD curl --fail http://localhost:${PORT}/health || exit 1


# Command to run the executable
CMD ["./jsonSiteGoServer"]
